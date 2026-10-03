package browser

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"net/url"
	"strings"
)

type Product struct {
	ProductID           string          `json:"source_product_id"`
	OfferID             string          `json:"source_offer_id,omitempty"`
	URL                 string          `json:"source_url,omitempty"`
	Title               string          `json:"title"`
	PriceMinor          *int64          `json:"price_minor,omitempty"`
	ReferencePriceMinor *int64          `json:"reference_price_minor,omitempty"`
	Currency            string          `json:"currency"`
	SellerID            string          `json:"seller_source_id,omitempty"`
	SellerName          string          `json:"seller_name,omitempty"`
	ImageURL            string          `json:"image_url,omitempty"`
	Availability        string          `json:"availability,omitempty"`
	Rank                int             `json:"rank,omitempty"`
	Raw                 json.RawMessage `json:"raw_metadata"`
}

type PageInfo struct {
	Page      int
	PageCount int
	HasNext   *bool
}
type ParseResult struct {
	Products []Product
	RawRows  int
	Page     PageInfo
}

func ParseResponse(body []byte) (ParseResult, error) {
	var root any
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	if err := dec.Decode(&root); err != nil {
		return ParseResult{}, fmt.Errorf("decode source response: %w", err)
	}
	var out ParseResult
	seen := map[string]bool{}
	var walk func(any, int)
	walk = func(v any, depth int) {
		if depth > 40 {
			return
		}
		switch x := v.(type) {
		case []any:
			for _, child := range x {
				walk(child, depth+1)
			}
		case map[string]any:
			readPage(x, &out.Page)
			if likelyProduct(x) {
				out.RawRows++
				p := parseProduct(x)
				key := p.ProductID + "\x00" + p.OfferID + "\x00" + string(p.Raw)
				if !seen[key] {
					seen[key] = true
					out.Products = append(out.Products, p)
				}
			}
			for _, child := range x {
				walk(child, depth+1)
			}
		}
	}
	walk(root, 0)
	return out, nil
}
func likelyProduct(m map[string]any) bool {
	id := field(m, "oskuId", "oskuID", "marketSku", "marketSkuId", "skuId", "productId", "modelId", "offerId")
	title := field(m, "title", "name", "wareTitle")
	return title != "" && (id != "" || (field(m, "url", "productUrl", "wareUrl", "href", "link") != "" && (money(m["price"]) != nil || money(m["priceValue"]) != nil)))
}
func parseProduct(m map[string]any) Product {
	raw, _ := json.Marshal(m)
	p := Product{
		ProductID:    field(m, "oskuId", "oskuID", "marketSku", "marketSkuId", "skuId", "productId", "modelId"),
		OfferID:      field(m, "offerId", "wareId"),
		Title:        field(m, "title", "name", "wareTitle"),
		URL:          field(m, "url", "productUrl", "wareUrl", "href", "link"),
		SellerID:     field(m, "sellerId", "shopId", "businessId"),
		SellerName:   field(m, "sellerName", "shopName", "businessName"),
		ImageURL:     field(m, "imageUrl", "image", "picture"),
		Availability: field(m, "availability", "stockStatus"),
		Currency:     field(m, "currency", "currencyCode"),
		Rank:         integer(m["rank"]),
		Raw:          raw,
	}
	if p.Rank == 0 {
		p.Rank = integer(m["position"])
	}
	if p.ProductID == "" {
		p.ProductID = p.OfferID
	}
	if p.ProductID == "" {
		h := sha256.Sum256([]byte(p.URL + "\x00" + p.Title))
		p.ProductID = "derived-" + hex.EncodeToString(h[:12])
	}
	if p.Currency == "" {
		p.Currency = "RUB"
	}
	if p.SellerName == "" {
		if shop, ok := m["shop"].(map[string]any); ok {
			p.SellerName = field(shop, "name", "title")
		}
	}
	if p.ImageURL == "" {
		if images, ok := m["pictures"].([]any); ok && len(images) > 0 {
			if img, ok := images[0].(map[string]any); ok {
				p.ImageURL = field(img, "url", "src")
			}
		}
	}
	if p.URL != "" {
		if u, err := url.Parse(p.URL); err == nil && u.Host == "" {
			base, _ := url.Parse("https://market.yandex.ru")
			p.URL = base.ResolveReference(u).String()
		}
	}
	// Yandex source prices are rubles; integer minor units retain exact decimals.
	p.PriceMinor = money(m["price"])
	if p.PriceMinor == nil {
		for _, k := range []string{"currentPrice", "priceValue", "priceData"} {
			if v := money(m[k]); v != nil {
				p.PriceMinor = v
				break
			}
		}
	}
	for _, k := range []string{"oldPrice", "referencePrice", "basePrice"} {
		if v := money(m[k]); v != nil {
			p.ReferencePriceMinor = v
			break
		}
	}
	if arr, ok := m["additionalPrices"].([]any); ok {
		for _, v := range arr {
			a, ok := v.(map[string]any)
			if !ok {
				continue
			}
			switch strings.ToLower(field(a, "priceType", "type")) {
			case "withdiscount":
				if z := money(a["priceValue"]); z != nil {
					p.PriceMinor = z
				}
			case "base", "oldprice", "withoutdiscount", "reference":
				if z := money(a["priceValue"]); z != nil {
					p.ReferencePriceMinor = z
				}
			}
		}
	}
	return p
}
func field(m map[string]any, keys ...string) string {
	for _, k := range keys {
		switch v := m[k].(type) {
		case string:
			if s := strings.TrimSpace(v); s != "" {
				return s
			}
		case json.Number:
			return v.String()
		}
	}
	return ""
}
func money(v any) *int64 {
	if obj, ok := v.(map[string]any); ok {
		for _, k := range []string{"value", "amount", "priceValue", "price"} {
			if x := money(obj[k]); x != nil {
				return x
			}
		}
		return nil
	}
	var s string
	switch x := v.(type) {
	case json.Number:
		s = x.String()
	case string:
		s = x
	default:
		return nil
	}
	s = strings.NewReplacer(" ", "", "\u00a0", "", "₽", "", ",", ".").Replace(strings.TrimSpace(s))
	r, ok := new(big.Rat).SetString(s)
	if !ok || r.Sign() < 0 {
		return nil
	}
	r.Mul(r, big.NewRat(100, 1))
	if !r.IsInt() || !r.Num().IsInt64() {
		return nil
	}
	n := r.Num().Int64()
	return &n
}
func readPage(m map[string]any, p *PageInfo) {
	hasPagination := false
	for _, key := range []string{"page", "pageNum", "currentPage", "pageCount", "pagesCount", "totalPages", "itemsPerPage", "pageSize"} {
		if _, ok := m[key]; ok {
			hasPagination = true
			break
		}
	}
	if !hasPagination {
		return
	}
	for _, k := range []string{"page", "pageNum", "currentPage"} {
		if n := integer(m[k]); n > p.Page {
			p.Page = n
		}
	}
	for _, k := range []string{"pageCount", "pagesCount", "totalPages"} {
		if n := integer(m[k]); n > p.PageCount {
			p.PageCount = n
		}
	}
	for _, k := range []string{"hasNextPage", "has_next_page", "hasNext"} {
		if b, ok := m[k].(bool); ok {
			p.HasNext = &b
		}
	}
}
func integer(v any) int {
	if n, ok := v.(json.Number); ok {
		var i int
		fmt.Sscan(n.String(), &i)
		return i
	}
	return 0
}
