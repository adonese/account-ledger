package ledger_test

import (
	"math"
	"reflect"
	"testing"

	ledger "github.com/adonese/account-ledger"
)

func TestMoney_Decimal(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		want string
		have ledger.Money
	}{
		{"format an amount", "-30.01", ledger.Money{Currency: ledger.AED, Minor: -3001}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.have.Decimal()
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Decimal() = %v, want %v", got, tt.want)
			}

		})
	}
}

func TestMoney_String(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		want string
		have ledger.Money
	}{
		{"pretty print money struct", "AED -30.01", ledger.Money{Currency: ledger.AED, Minor: -3001}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.have.String()
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Decimal() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMoney_Add(t *testing.T) {
	tests := []struct {
		name  string // description of this test case
		want  ledger.Money
		have  ledger.Money
		other ledger.Money
	}{
		{"add two amounts", ledger.Money{Currency: ledger.AED, Minor: 2000}, ledger.Money{Currency: ledger.AED, Minor: 1000}, ledger.Money{Currency: ledger.AED, Minor: 1000}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.have.Add(tt.other)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Decimal() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMoney_AddPanics(t *testing.T) {
	tests := []struct {
		name string
		a, b ledger.Money
	}{
		{"different currencies", ledger.Money{Currency: ledger.AED, Minor: 1000}, ledger.Money{Currency: ledger.BHD, Minor: 1000}},
		{"overflow", ledger.Money{Currency: ledger.AED, Minor: math.MaxInt64}, ledger.Money{Currency: ledger.AED, Minor: 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Error("Add() did not panic")
				}
			}()
			tt.a.Add(tt.b)
		})
	}
}

func TestMoney_Neg(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		want ledger.Money
		have ledger.Money
	}{
		{"neg neg would be a positive amount", ledger.Money{ledger.AED, 3001}, ledger.Money{Currency: ledger.AED, Minor: -3001}},
		{"negative amount", ledger.Money{ledger.AED, -3001}, ledger.Money{Currency: ledger.AED, Minor: 3001}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.have.Neg()
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Decimal() = %v, want %v", got, tt.want)
			}
		})
	}
}
