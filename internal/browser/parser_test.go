package browser

import (
	"testing"
)

func TestParseResponse(t *testing.T) {
	r, e := ParseResponse([]byte(`{"page":1,"pageCount":2,"items":[{"oskuId":"42","offerId":"offer","title":"Widget","price":"1234.50","oldPrice":1500,"sellerName":"Shop","extra":{"badge":"new"}},{"oskuId":"42","offerId":"offer","title":"Widget","price":"1234.50","oldPrice":1500,"sellerName":"Shop","extra":{"badge":"new"}}]}`))
	if e != nil {
		t.Fatal(e)
	}
	if r.RawRows != 2 || len(r.Products) != 1 || *r.Products[0].PriceMinor != 123450 || r.Page.PageCount != 2 {
		t.Fatalf("parsed %+v", r)
	}
}
func TestMalformedResponse(t *testing.T) {
	if _, e := ParseResponse([]byte("{")); e == nil {
		t.Fatal("expected error")
	}
}
func TestChallenge(t *testing.T) {
	d := ClassifyChallenge([]byte("<title>Captcha</title><div class='smart-captcha'></div>"), "https://market.yandex.ru/showcaptcha")
	if d.Class != ChallengeHard {
		t.Fatal(d)
	}
}
