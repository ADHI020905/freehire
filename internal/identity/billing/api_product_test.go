package billing

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
)

const (
	apiPrice     = "price_api_monthly"
	apiDataPrice = "price_api_data_monthly"
)

// apiConfig sells all four prices, the shape production runs once the API product exists.
func apiConfig() Config {
	return Config{
		Prices:        []string{proPrice},
		UltraPrices:   []string{ultraPrice},
		APIPrices:     []string{apiPrice},
		APIDataPrices: []string{apiDataPrice},
	}
}

func TestConfigFromEnvReadsAPIPrices(t *testing.T) {
	t.Setenv("STRIPE_API_PRICE_IDS", " price_a , price_b ")
	t.Setenv("STRIPE_API_DATA_PRICE_IDS", "price_d")
	cfg := ConfigFromEnv()
	if len(cfg.APIPrices) != 2 || cfg.APIPrices[0] != "price_a" || cfg.APIPrices[1] != "price_b" {
		t.Fatalf("APIPrices = %q", cfg.APIPrices)
	}
	if len(cfg.APIDataPrices) != 1 || cfg.APIDataPrices[0] != "price_d" {
		t.Fatalf("APIDataPrices = %q", cfg.APIDataPrices)
	}
}

func TestSellsAPIPrices(t *testing.T) {
	cfg := apiConfig()
	for _, p := range []string{proPrice, ultraPrice, apiPrice, apiDataPrice} {
		if !cfg.Sells(p) {
			t.Errorf("Sells(%q) = false, want true", p)
		}
	}
	if cfg.Sells("price_unknown") {
		t.Error("Sells(unknown) = true")
	}
}

// TestEntitlementFromKeepsProductsApart: one re-read resolves all four tiers, and an API
// subscription confers no plan tier while a Pro one confers no API tier.
func TestEntitlementFromKeepsProductsApart(t *testing.T) {
	end := at(t, "2026-11-19T00:00:00Z")
	live := func(price string) subscription {
		return subscription{Status: "active", CurrentPeriodEnd: end, PriceIDs: []string{price}}
	}
	cfg := apiConfig()

	cases := []struct {
		name string
		sub  subscriber
		want entitlement
	}{
		{"api only", sub(live(apiPrice)), entitlement{API: end}},
		{"data only", sub(live(apiDataPrice)), entitlement{Data: end}},
		{"pro only", sub(live(proPrice)), entitlement{Pro: end}},
		{"pro and api", sub(live(proPrice), live(apiPrice)), entitlement{Pro: end, API: end}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := entitlementFrom(tc.sub, cfg); got != tc.want {
				t.Fatalf("entitlementFrom = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// TestCheckoutKeepsProductFamiliesApart is the bug this change must not ship: checkout
// changes the customer's entitling subscription IN PLACE when the requested price differs,
// so without a family split a Pro subscriber buying API would have Pro replaced by it.
func TestCheckoutKeepsProductFamiliesApart(t *testing.T) {
	cfg := apiConfig()
	end := at(t, "2026-11-19T00:00:00Z")
	live := func(id, price string) subscription {
		return subscription{ID: id, ItemID: id + "_item", Status: "active", CurrentPeriodEnd: end, PriceIDs: []string{price}}
	}

	cases := []struct {
		name      string
		sub       subscriber
		requested string
		want      checkoutTarget
	}{
		{"pro subscriber buying api opens a new checkout", sub(live("sub_pro", proPrice)), apiPrice, checkoutTarget{}},
		{"api subscriber buying pro opens a new checkout", sub(live("sub_api", apiPrice)), proPrice, checkoutTarget{}},
		{"api subscriber buying data upgrades in place", sub(live("sub_api", apiPrice)), apiDataPrice,
			checkoutTarget{SubscriptionID: "sub_api", ItemID: "sub_api_item"}},
		{"pro and api subscriber buying ultra changes only pro", sub(live("sub_pro", proPrice), live("sub_api", apiPrice)), ultraPrice,
			checkoutTarget{SubscriptionID: "sub_pro", ItemID: "sub_pro_item"}},
		{"api subscriber re-buying api is a no-op", sub(live("sub_api", apiPrice)), apiPrice, checkoutTarget{AlreadyOnPrice: true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, b := cfg.checkoutFamily(tc.requested)
			if got := decideCheckoutTarget(tc.sub, tc.requested, a, b); got != tc.want {
				t.Fatalf("target = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestPublicPricesTagsAPITiers(t *testing.T) {
	s, _ := pricedService(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		id := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
		_, _ = w.Write([]byte(`{"id":"` + id + `","unit_amount":2900,"currency":"usd","recurring":{"interval":"month"}}`))
	})
	s.cfg.APIPrices = []string{apiPrice}
	s.cfg.APIDataPrices = []string{apiDataPrice}

	tiers := map[string]string{}
	for _, p := range s.PublicPrices(context.Background()) {
		tiers[p.ID] = p.Tier
	}
	if tiers[apiPrice] != "api" || tiers[apiDataPrice] != "api_data" || tiers["price_pro_monthly"] != "pro" {
		t.Fatalf("tiers = %v", tiers)
	}
}

func TestAPITierOf(t *testing.T) {
	now := at(t, "2026-10-19T12:00:00Z")
	future, past := now.Add(time.Hour), now.Add(-time.Hour)
	cases := []struct {
		name      string
		api, data time.Time
		want      APITier
	}{
		{"nothing", time.Time{}, time.Time{}, APITierNone},
		{"api live", future, time.Time{}, APITierAPI},
		{"data live", time.Time{}, future, APITierData},
		{"both live resolves to data", future, future, APITierData},
		{"data lapsed, api live", future, past, APITierAPI},
		{"both lapsed", past, past, APITierNone},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := APITierOf(tc.api, tc.data, now); got != tc.want {
				t.Fatalf("APITierOf = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestPublicPricesSurviveABrokenAPIPrice(t *testing.T) {
	s, _ := pricedService(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/price_typo") {
			http.Error(w, `{"error":{"message":"No such price"}}`, http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(monthlyPriceBody))
	})
	s.cfg.APIPrices = []string{"price_typo"}

	got := s.PublicPrices(context.Background())
	if len(got) != 1 || got[0].Tier != "pro" {
		t.Fatalf("prices = %+v, want Pro still listed and the broken API product omitted", got)
	}
}
