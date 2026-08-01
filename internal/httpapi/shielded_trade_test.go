package httpapi

import (
	"math/big"
	"testing"

	"github.com/iden3/go-iden3-crypto/poseidon"
)

func TestShieldedTradeDenominationIncludesExactFee(t *testing.T) {
	if actual := shieldedTradeDenominationRaw("10", 50, 18); actual != "10050000000000000000" {
		t.Fatalf("expected 10.05 BUDOL base units, got %s", actual)
	}
}

func TestShieldedOrderCommitmentChangesWithSideAndBatch(t *testing.T) {
	market := big.NewInt(11)
	amount := big.NewInt(10_000_000)
	salt := big.NewInt(22)
	batch := big.NewInt(33)
	yes, err := poseidon.Hash([]*big.Int{market, big.NewInt(1), amount, salt, batch})
	if err != nil {
		t.Fatal(err)
	}
	no, err := poseidon.Hash([]*big.Int{market, big.NewInt(2), amount, salt, batch})
	if err != nil {
		t.Fatal(err)
	}
	nextBatch, err := poseidon.Hash([]*big.Int{market, big.NewInt(1), amount, salt, big.NewInt(34)})
	if err != nil {
		t.Fatal(err)
	}
	if yes.Cmp(no) == 0 || yes.Cmp(nextBatch) == 0 {
		t.Fatal("order commitment must bind side and batch")
	}
}
