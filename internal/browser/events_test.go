package browser

import (
	"testing"
)

func TestTabGenerationAndOverflow(t *testing.T) {
	tab := &tab{events: make(chan event, 1)}
	tab.begin()
	tab.publish(0, event{raw: 1})
	if len(tab.events) != 0 {
		t.Fatal("stale event accepted")
	}
	tab.publish(1, event{raw: 1})
	tab.publish(1, event{raw: 2})
	if !tab.overflow.Load() {
		t.Fatal("overflow not reported")
	}
	tab.begin()
	if tab.overflow.Load() || len(tab.events) != 0 {
		t.Fatal("navigation state not cleared")
	}
}
func TestYandexHost(t *testing.T) {
	if !yandexHost("market.yandex.ru") || yandexHost("evilyandex.ru") || yandexHost("yandex.ru.evil.example") {
		t.Fatal("host boundary")
	}
}
func TestProductWithoutSourceID(t *testing.T) {
	r, e := ParseResponse([]byte(`{"items":[{"title":"Widget","url":"/product/thing","price":"10.25"}]}`))
	if e != nil || len(r.Products) != 1 || r.Products[0].ProductID == "" || *r.Products[0].PriceMinor != 1025 {
		t.Fatalf("parsed %+v %v", r, e)
	}
}
