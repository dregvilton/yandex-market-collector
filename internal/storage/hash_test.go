package storage

import (
	"encoding/json"
	"testing"

	"github.com/dregvilton/yandex-market-collector/internal/browser"
)

func TestContentHash(t *testing.T) {
	price := int64(10000)
	p := browser.Product{ProductID: "a", Title: "Widget", PriceMinor: &price, Raw: json.RawMessage(`{"badge":"new","requestId":"ephemeral"}`)}
	first := contentHash(p)
	p.Rank = 99
	p.Raw = json.RawMessage(`{"badge":"new","requestId":"another"}`)
	if contentHash(p) != first {
		t.Fatal("rank or request metadata changed hash")
	}
	price = 9000
	if contentHash(p) == first {
		t.Fatal("price change ignored")
	}
	price = 10000
	p.Raw = json.RawMessage(`{"badge":"sale"}`)
	if contentHash(p) == first {
		t.Fatal("source badge change ignored")
	}
}
