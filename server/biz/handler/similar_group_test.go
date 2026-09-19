package handler

import (
	"strings"
	"testing"

	"aaa_word/biz/model"
)

func TestNormalizeSimilarGroupWord(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
		ok   bool
	}{
		{name: "plain", raw: "affect", want: "affect", ok: true},
		{name: "trim and lowercase", raw: "  AfFeCt  ", want: "affect", ok: true},
		{name: "hyphen", raw: "well-being", want: "well-being", ok: true},
		{name: "apostrophe", raw: "can't", want: "can't", ok: true},
		{name: "empty", raw: "  ", ok: false},
		{name: "phrase", raw: "look up", ok: false},
		{name: "number", raw: "word2", ok: false},
		{name: "symbol", raw: "word!", ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := normalizeSimilarGroupWord(tt.raw)
			if got != tt.want || ok != tt.ok {
				t.Fatalf("normalizeSimilarGroupWord(%q) = (%q, %v), want (%q, %v)",
					tt.raw, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestNormalizeWordAssociationRequest(t *testing.T) {
	maxHint := strings.Repeat("意", 200)
	tests := []struct {
		name string
		req  model.WordAssociationRequest
		want model.WordAssociationRequest
		ok   bool
	}{
		{
			name: "normalizes valid request",
			req: model.WordAssociationRequest{
				Word: "  AfFeCt  ", MeaningHint: "  情感  ", SpellingHint: "  aff  ",
			},
			want: model.WordAssociationRequest{
				Word: "affect", MeaningHint: "情感", SpellingHint: "aff",
			},
			ok: true,
		},
		{
			name: "allows empty hints",
			req:  model.WordAssociationRequest{Word: "affect"},
			want: model.WordAssociationRequest{Word: "affect"},
			ok:   true,
		},
		{
			name: "allows 200 runes",
			req:  model.WordAssociationRequest{Word: "affect", MeaningHint: maxHint},
			want: model.WordAssociationRequest{Word: "affect", MeaningHint: maxHint},
			ok:   true,
		},
		{
			name: "rejects phrase",
			req:  model.WordAssociationRequest{Word: "look up"},
		},
		{
			name: "rejects long meaning hint",
			req:  model.WordAssociationRequest{Word: "affect", MeaningHint: maxHint + "意"},
		},
		{
			name: "rejects long spelling hint",
			req:  model.WordAssociationRequest{Word: "affect", SpellingHint: maxHint + "意"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := normalizeWordAssociationRequest(tt.req)
			if got != tt.want || ok != tt.ok {
				t.Fatalf("normalizeWordAssociationRequest() = (%+v, %v), want (%+v, %v)",
					got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestNormalizeSimilarGroupMemberAddRequest(t *testing.T) {
	tests := []struct {
		name string
		req  model.SimilarGroupMemberAddRequest
		want model.SimilarGroupMemberAddRequest
		ok   bool
	}{
		{
			name: "normalizes valid words",
			req:  model.SimilarGroupMemberAddRequest{Word: " Affect ", CandidateWord: " EFFECT "},
			want: model.SimilarGroupMemberAddRequest{Word: "affect", CandidateWord: "effect"},
			ok:   true,
		},
		{
			name: "allows hyphen and apostrophe",
			req:  model.SimilarGroupMemberAddRequest{Word: "well-being", CandidateWord: "can't"},
			want: model.SimilarGroupMemberAddRequest{Word: "well-being", CandidateWord: "can't"},
			ok:   true,
		},
		{
			name: "rejects same word",
			req:  model.SimilarGroupMemberAddRequest{Word: " Affect ", CandidateWord: "affect"},
		},
		{
			name: "rejects phrase",
			req:  model.SimilarGroupMemberAddRequest{Word: "look up", CandidateWord: "lookup"},
		},
		{
			name: "rejects invalid candidate",
			req:  model.SimilarGroupMemberAddRequest{Word: "affect", CandidateWord: "effect2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := normalizeSimilarGroupMemberAddRequest(tt.req)
			if got != tt.want || ok != tt.ok {
				t.Fatalf("normalizeSimilarGroupMemberAddRequest() = (%+v, %v), want (%+v, %v)",
					got, ok, tt.want, tt.ok)
			}
		})
	}
}
