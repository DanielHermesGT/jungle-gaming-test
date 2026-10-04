package wager_test

import (
	"testing"

	"github.com/DanielHermesGT/jungle-gaming-test/internal/domain/money"
	domainwager "github.com/DanielHermesGT/jungle-gaming-test/internal/domain/wager"
	usecasewager "github.com/DanielHermesGT/jungle-gaming-test/internal/usecase/wager"
)

func TestCanonicalPayloadHashStable(t *testing.T) {
	amt, err := money.Parse("BRL", "25.00")
	if err != nil {
		t.Fatal(err)
	}
	f := usecasewager.PayloadFields{
		ProviderID:            "prov-a",
		ExternalTransactionID: "tx-1",
		PlayerID:              "p-1",
		WalletID:              "w-1",
		RoundID:               "r-1",
		GameID:                "g-1",
		Kind:                  domainwager.KindBet,
		Amount:                amt,
	}
	h1, err := usecasewager.CanonicalPayloadHash(f)
	if err != nil {
		t.Fatal(err)
	}
	h2, err := usecasewager.CanonicalPayloadHash(f)
	if err != nil || h1 != h2 || len(h1) != 64 {
		t.Fatalf("h1=%s h2=%s", h1, h2)
	}
	f.Amount, _ = money.Parse("BRL", "26.00")
	h3, err := usecasewager.CanonicalPayloadHash(f)
	if err != nil || h3 == h1 {
		t.Fatalf("amount change must change hash")
	}
}
