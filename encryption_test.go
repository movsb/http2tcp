package main

import (
	"bytes"
	"fmt"
	"testing"
)

func TestAesGem(t *testing.T) {
	p1 := NewPrivateKey()
	p2 := NewPrivateKey()

	sh1 := p1.SharedSecret(p2.PublicKey())
	sh2 := p2.SharedSecret(p1.PublicKey())
	if sh1 != sh2 {
		panic("sh1 != sh2")
	}

	g, err := NewAesGcm(sh1)
	if err != nil {
		panic(err)
	}

	plaintext := []byte(`🍑 & 🍐`)
	encrypted, err := g.Encrypt(plaintext)
	if err != nil {
		panic(err)
	}
	decrypted, err := g.Decrypt(encrypted)
	if err != nil {
		panic(err)
	}
	fmt.Println(`plaintext:`, plaintext)
	fmt.Println(`encrypted:`, encrypted)
	fmt.Println(`decrypted:`, decrypted)
	if !bytes.Equal(decrypted, plaintext) {
		panic(`not equal`)
	}
	fmt.Println(`equal`)
}
