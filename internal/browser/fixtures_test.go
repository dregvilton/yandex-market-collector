package browser

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSourceFixtures(t *testing.T) {
	cases := []struct {
		name string
		hard bool
	}{{"real_showcaptcha.html", true}, {"visible_captcha.html", true}, {"normal_product_harmless_challenge.json", false}, {"product_lazy_challenge.json", true}, {"background_service_challenge.json", true}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body, e := os.ReadFile(filepath.Join("testdata", tc.name))
			if e != nil {
				t.Fatal(e)
			}
			if got := hardChallenge(body, "https://market.yandex.ru/search"); got != tc.hard {
				t.Fatalf("hard=%t", got)
			}
		})
	}
}
func TestParsedSourceFixture(t *testing.T) {
	body, e := os.ReadFile("testdata/normal_product_harmless_challenge.json")
	if e != nil {
		t.Fatal(e)
	}
	r, e := ParseResponse(body)
	if e != nil {
		t.Fatal(e)
	}
	if r.RawRows != 1 || len(r.Products) != 1 || r.Page.PageCount != 10 || r.Products[0].PriceMinor == nil || *r.Products[0].PriceMinor != 500000 {
		t.Fatalf("parsed %+v", r)
	}
}
