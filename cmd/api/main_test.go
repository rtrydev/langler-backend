package main

import "testing"

func TestEmbeddingIndexURLs(t *testing.T) {
	t.Parallel()

	t.Run("per language configuration", func(t *testing.T) {
		t.Parallel()

		urls, err := embeddingIndexURLs(`{"ja":"https://cdn/ja.embed","pl":"https://cdn/pl.embed"}`, "")
		if err != nil {
			t.Fatalf("embeddingIndexURLs: %v", err)
		}
		if urls["ja"] != "https://cdn/ja.embed" || urls["pl"] != "https://cdn/pl.embed" {
			t.Fatalf("urls = %v", urls)
		}
	})

	t.Run("legacy Japanese configuration", func(t *testing.T) {
		t.Parallel()

		urls, err := embeddingIndexURLs("", "https://cdn/ja.embed")
		if err != nil {
			t.Fatalf("embeddingIndexURLs: %v", err)
		}
		if len(urls) != 1 || urls["ja"] != "https://cdn/ja.embed" {
			t.Fatalf("urls = %v", urls)
		}
	})

	t.Run("unknown language", func(t *testing.T) {
		t.Parallel()

		if _, err := embeddingIndexURLs(`{"xx":"https://cdn/xx.embed"}`, ""); err == nil {
			t.Fatal("embeddingIndexURLs error = nil")
		}
	})
}
