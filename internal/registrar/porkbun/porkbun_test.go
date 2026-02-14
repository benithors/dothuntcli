package porkbun

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestClient_CheckDomain_Success(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method=%q, want POST", r.Method)
		}
		if !strings.HasPrefix(r.URL.Path, "/domain/checkDomain/") {
			t.Fatalf("path=%q, want /domain/checkDomain/...", r.URL.Path)
		}

		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["apikey"] != "k" || body["secretapikey"] != "s" {
			t.Fatalf("bad keys in body: %#v", body)
		}

		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{
			"status":"SUCCESS",
			"response":{
				"avail":"yes",
				"price":"10.29",
				"regularPrice":"10.29",
				"premium":"no",
				"minDuration":1,
				"firstYearPromo":"no"
			},
			"limits":{"TTL":"10","limit":"100","used":1,"naturalLanguage":"example"}
		}`))
	}))
	defer srv.Close()

	c, err := NewClient(Options{
		APIKey:        "k",
		SecretAPIKey:  "s",
		BaseURL:       srv.URL,
		Timeout:       2 * time.Second,
		MinDelay:      1 * time.Nanosecond,
		MaxConcurrent: 1,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	got, err := c.CheckDomain(context.Background(), "example.com")
	if err != nil {
		t.Fatalf("CheckDomain: %v", err)
	}
	if !got.Buyable {
		t.Fatalf("Buyable=false, want true")
	}
	if got.Premium {
		t.Fatalf("Premium=true, want false")
	}
	if got.Price != "10.29" {
		t.Fatalf("Price=%q, want 10.29", got.Price)
	}
	if got.MinDuration != 1 {
		t.Fatalf("MinDuration=%d, want 1", got.MinDuration)
	}
	if got.Limits == nil || got.Limits.TTLSeconds != 10 || got.Limits.Limit != 100 || got.Limits.Used != 1 {
		t.Fatalf("Limits=%#v, want parsed", got.Limits)
	}
}

func TestClient_CheckDomain_ErrorStatus(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ERROR","message":"nope"}`))
	}))
	defer srv.Close()

	c, err := NewClient(Options{
		APIKey:        "k",
		SecretAPIKey:  "s",
		BaseURL:       srv.URL,
		MinDelay:      1 * time.Nanosecond,
		MaxConcurrent: 1,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	_, err = c.CheckDomain(context.Background(), "example.com")
	if err == nil || !strings.Contains(err.Error(), "nope") {
		t.Fatalf("err=%v, want message", err)
	}
}
