package export

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/dregvilton/yandex-market-collector/internal/storage"
)

type Options struct {
	Format, Target string
	Since, Until   *time.Time
}
type Row struct {
	ID                  int64           `json:"id"`
	Target              string          `json:"target"`
	ObservedAt          time.Time       `json:"observed_at"`
	ProductID           string          `json:"source_product_id"`
	OfferID             string          `json:"source_offer_id"`
	URL                 string          `json:"source_url"`
	Title               string          `json:"title"`
	PriceMinor          *int64          `json:"price_minor"`
	ReferencePriceMinor *int64          `json:"reference_price_minor"`
	Currency            string          `json:"currency"`
	SellerID            string          `json:"seller_source_id"`
	SellerName          string          `json:"seller_name"`
	Availability        string          `json:"availability"`
	ImageURL            string          `json:"image_url"`
	Raw                 json.RawMessage `json:"raw_metadata"`
}

func Write(ctx context.Context, s *storage.Store, w io.Writer, o Options) error {
	if o.Format != "csv" && o.Format != "jsonl" {
		return errors.New("format must be csv or jsonl")
	}
	rows, e := s.DB.Query(ctx, `SELECT o.id,t.key,o.observed_at,o.source_product_id,o.source_offer_id,o.source_url,o.title,o.price_minor,o.reference_price_minor,o.currency,o.seller_source_id,o.seller_name,o.availability,o.image_url,o.raw_metadata FROM observations o JOIN collection_targets t ON t.id=o.target_id WHERE ($1='' OR t.key=$1) AND ($2::timestamptz IS NULL OR o.observed_at >= $2) AND ($3::timestamptz IS NULL OR o.observed_at < $3) ORDER BY o.observed_at,o.id`, o.Target, o.Since, o.Until)
	if e != nil {
		return e
	}
	defer rows.Close()
	csvw := csv.NewWriter(w)
	if o.Format == "csv" {
		if e = csvw.Write([]string{"id", "target", "observed_at", "source_product_id", "source_offer_id", "source_url", "title", "price_minor", "reference_price_minor", "currency", "seller_source_id", "seller_name", "availability", "image_url", "raw_metadata"}); e != nil {
			return e
		}
	}
	enc := json.NewEncoder(w)
	for rows.Next() {
		var r Row
		if e = rows.Scan(&r.ID, &r.Target, &r.ObservedAt, &r.ProductID, &r.OfferID, &r.URL, &r.Title, &r.PriceMinor, &r.ReferencePriceMinor, &r.Currency, &r.SellerID, &r.SellerName, &r.Availability, &r.ImageURL, &r.Raw); e != nil {
			return e
		}
		if o.Format == "jsonl" {
			if e = enc.Encode(r); e != nil {
				return e
			}
		} else {
			e = csvw.Write([]string{strconv.FormatInt(r.ID, 10), r.Target, r.ObservedAt.Format(time.RFC3339Nano), r.ProductID, r.OfferID, r.URL, r.Title, optional(r.PriceMinor), optional(r.ReferencePriceMinor), r.Currency, r.SellerID, r.SellerName, r.Availability, r.ImageURL, string(r.Raw)})
			if e != nil {
				return e
			}
		}
	}
	if e = rows.Err(); e != nil {
		return e
	}
	if o.Format == "csv" {
		csvw.Flush()
		return csvw.Error()
	}
	return nil
}
func optional(p *int64) string {
	if p == nil {
		return ""
	}
	return strconv.FormatInt(*p, 10)
}
func File(ctx context.Context, s *storage.Store, path string, o Options) error {
	if path == "-" {
		return Write(ctx, s, os.Stdout, o)
	}
	f, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return fmt.Errorf("create export: %w", e)
	}
	defer f.Close()
	if e = Write(ctx, s, f, o); e != nil {
		_ = os.Remove(path)
		return e
	}
	if e = f.Sync(); e != nil {
		_ = os.Remove(path)
		return e
	}
	return nil
}
