package attaggregation

import (
	"bytes"
	"sort"
	"testing"

	ethpb "github.com/prysmaticlabs/ethereumapis/eth/v1alpha1"
	"github.com/prysmaticlabs/go-bitfield"
	"github.com/prysmaticlabs/go-ssz"
)

func TestAggregate_AggregatePair(t *testing.T) {
	tests := []struct {
		a1   *ethpb.Attestation
		a2   *ethpb.Attestation
		want *ethpb.Attestation
	}{
		{
			a1:   &ethpb.Attestation{AggregationBits: []byte{}},
			a2:   &ethpb.Attestation{AggregationBits: []byte{}},
			want: &ethpb.Attestation{AggregationBits: []byte{}},
		},
		{
			a1:   &ethpb.Attestation{AggregationBits: bitfield.Bitlist{0x03}},
			a2:   &ethpb.Attestation{AggregationBits: bitfield.Bitlist{0x02}},
			want: &ethpb.Attestation{AggregationBits: []byte{0x03}},
		},
		{
			a1:   &ethpb.Attestation{AggregationBits: bitfield.Bitlist{0x02}},
			a2:   &ethpb.Attestation{AggregationBits: bitfield.Bitlist{0x03}},
			want: &ethpb.Attestation{AggregationBits: []byte{0x03}},
		},
	}
	for _, tt := range tests {
		got, err := AggregatePair(tt.a1, tt.a2)
		if err != nil {
			t.Fatal(err)
		}
		if !ssz.DeepEqual(got, tt.want) {
			t.Errorf("AggregatePair() = %v, want %v", got, tt.want)
		}
	}
}

func TestAggregate_AggregatePair_OverlapFails(t *testing.T) {
	tests := []struct {
		a1 *ethpb.Attestation
		a2 *ethpb.Attestation
	}{
		{
			a1: &ethpb.Attestation{AggregationBits: bitfield.Bitlist{0x1F}},
			a2: &ethpb.Attestation{AggregationBits: bitfield.Bitlist{0x11}},
		},
		{
			a1: &ethpb.Attestation{AggregationBits: bitfield.Bitlist{0xFF, 0x85}},
			a2: &ethpb.Attestation{AggregationBits: bitfield.Bitlist{0x13, 0x8F}},
		},
	}
	for _, tt := range tests {
		_, err := AggregatePair(tt.a1, tt.a2)
		if err != ErrBitsOverlap {
			t.Error("Did not receive wanted error")
		}
	}
}

func TestAggregate_AggregatePair_DiffLengthFails(t *testing.T) {
	tests := []struct {
		a1 *ethpb.Attestation
		a2 *ethpb.Attestation
	}{
		{
			a1: &ethpb.Attestation{AggregationBits: bitfield.Bitlist{0x0F}},
			a2: &ethpb.Attestation{AggregationBits: bitfield.Bitlist{0x11}},
		},
	}
	for _, tt := range tests {
		_, err := AggregatePair(tt.a1, tt.a2)
		if err != ErrBitsDifferentLen {
			t.Error("Did not receive wanted error")
		}
	}
}

func TestAggregate_Aggregate(t *testing.T) {
	// Each test defines the aggregation bitfield inputs and the wanted output result.
	tests := []struct {
		name   string
		inputs []bitfield.Bitlist
		want   []bitfield.Bitlist
	}{
		{
			name: "two attestations with no overlap",
			inputs: []bitfield.Bitlist{
				{0b00000001, 0b1},
				{0b00000010, 0b1},
			},
			want: []bitfield.Bitlist{
				{0b00000011, 0b1},
			},
		},
		{
			name:   "256 attestations with single bit set",
			inputs: bitlistsWithSingleBitSet(256),
			want: []bitfield.Bitlist{
				bitlistWithAllBitsSet(256),
			},
		},
		{
			name:   "1024 attestations with single bit set",
			inputs: bitlistsWithSingleBitSet(1024),
			want: []bitfield.Bitlist{
				bitlistWithAllBitsSet(1024),
			},
		},
		{
			name: "two attestations with overlap",
			inputs: []bitfield.Bitlist{
				{0b00000101, 0b1},
				{0b00000110, 0b1},
			},
			want: []bitfield.Bitlist{
				{0b00000101, 0b1},
				{0b00000110, 0b1},
			},
		},
		{
			name: "some attestations overlap",
			inputs: []bitfield.Bitlist{
				{0b00001001, 0b1},
				{0b00010110, 0b1},
				{0b00001010, 0b1},
				{0b00110001, 0b1},
			},
			want: []bitfield.Bitlist{
				{0b00111011, 0b1},
				{0b00011111, 0b1},
			},
		},
		{
			name: "some attestations produce duplicates which are removed",
			inputs: []bitfield.Bitlist{
				{0b00000101, 0b1},
				{0b00000110, 0b1},
				{0b00001010, 0b1},
				{0b00001001, 0b1},
			},
			want: []bitfield.Bitlist{
				{0b00001111, 0b1}, // both 0&1 and 2&3 produce this bitlist
			},
		},
		{
			name: "two attestations where one is fully contained within the other",
			inputs: []bitfield.Bitlist{
				{0b00000001, 0b1},
				{0b00000011, 0b1},
			},
			want: []bitfield.Bitlist{
				{0b00000011, 0b1},
			},
		},
		{
			name: "two attestations where one is fully contained within the other reversed",
			inputs: []bitfield.Bitlist{
				{0b00000011, 0b1},
				{0b00000001, 0b1},
			},
			want: []bitfield.Bitlist{
				{0b00000011, 0b1},
			},
		},
		{
			name: "attestations with different bitlist lengths",
			inputs: []bitfield.Bitlist{
				{0b00000011, 0b10},
				{0b00000111, 0b100},
				{0b00000100, 0b1},
			},
			want: []bitfield.Bitlist{
				{0b00000011, 0b10},
				{0b00000111, 0b100},
				{0b00000100, 0b1},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Aggregate(makeAttestationsFromBitlists(tt.inputs))
			if err != nil {
				t.Fatal(err)
			}
			sort.Slice(got, func(i, j int) bool {
				return got[i].AggregationBits.Bytes()[0] < got[j].AggregationBits.Bytes()[0]
			})
			sort.Slice(tt.want, func(i, j int) bool {
				return tt.want[i].Bytes()[0] < tt.want[j].Bytes()[0]
			})
			if len(got) != len(tt.want) {
				t.Logf("got=%v", got)
				t.Fatalf("Wrong number of responses. Got %d, wanted %d", len(got), len(tt.want))
			}
			for i, w := range tt.want {
				if !bytes.Equal(got[i].AggregationBits.Bytes(), w.Bytes()) {
					t.Errorf("Unexpected bitlist at index %d, got %b, wanted %b", i, got[i].AggregationBits.Bytes(), w.Bytes())
				}
			}
		})
	}
}
