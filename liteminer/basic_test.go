package liteminer

import "testing"

func TestBasic(t *testing.T) {
	p, err := CreatePool("3000")
	if err != nil {
		t.Errorf("Received error %v when creating pool", err)
	}

	addr := p.Addr.String()

	numMiners := 2
	miners := make([]*Miner, numMiners)
	for i := 0; i < numMiners; i++ {
		m, err := CreateMiner(addr)
		if err != nil {
			t.Errorf("Received error %v when creating miner", err)
		}
		miners[i] = m
	}

	client := CreateClient([]string{addr})

	data := "data"
	upperbound := uint64(1)
	nonces, err := client.Mine(data, upperbound)
	if err != nil {
		t.Errorf("Received error %v when mining", err)
	} else {
		for _, nonce := range nonces {
			expected := int64(1)
			if nonce != expected {
				t.Errorf("Expected nonce %d, but received %d", expected, nonce)
			}
		}
	}
}
