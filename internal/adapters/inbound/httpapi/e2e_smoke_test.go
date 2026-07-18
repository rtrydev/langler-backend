package httpapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"

	"github.com/rtrydev/langler-backend/internal/adapters/inbound/httpapi"
	"github.com/rtrydev/langler-backend/internal/adapters/outbound/dynamoref"
	appref "github.com/rtrydev/langler-backend/internal/application/reference"
	"github.com/rtrydev/langler-backend/internal/application/status"
)

func TestE2EAgainstLoadedReferenceData(t *testing.T) {
	endpoint := os.Getenv("DYNAMODB_LOCAL_ENDPOINT")
	table := os.Getenv("E2E_TABLE")
	if endpoint == "" || table == "" {
		t.Skip("set DYNAMODB_LOCAL_ENDPOINT and E2E_TABLE to run")
	}

	client := dynamodb.New(dynamodb.Options{
		Region:       "us-east-1",
		Credentials:  credentials.NewStaticCredentialsProvider("local", "local", ""),
		BaseEndpoint: aws.String(endpoint),
	})
	repo, err := dynamoref.NewRepository(client, table)
	if err != nil {
		t.Fatalf("NewRepository: %v", err)
	}
	refSvc, err := appref.NewService(repo)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	statusSvc, err := status.NewService("langler-backend", "local")
	if err != nil {
		t.Fatalf("status.NewService: %v", err)
	}
	h, err := httpapi.NewHandler(statusSvc, refSvc)
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	ctx := context.Background()

	call := func(path string, params map[string]string) map[string]any {
		t.Helper()
		resp, err := h.Handle(ctx, getRequest(path, params))
		if err != nil {
			t.Fatalf("Handle %s: %v", path, err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s %v: status %d body %s", path, params, resp.StatusCode, resp.Body)
		}
		var body map[string]any
		if err := json.Unmarshal([]byte(resp.Body), &body); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		return body
	}

	for _, level := range []string{"N5", "N4", "N3", "N2", "N1"} {
		body := call("/reference/vocab", map[string]string{"lang": "ja", "level": level, "limit": "5"})
		items := body["items"].([]any)
		if len(items) != 5 {
			t.Fatalf("vocab %s: %d items, want 5", level, len(items))
		}
		first := items[0].(map[string]any)
		for _, field := range []string{"headword", "reading", "gloss", "level", "sourceId", "license"} {
			if first[field] == nil || first[field] == "" {
				t.Errorf("vocab %s: field %q missing in %v", level, field, first)
			}
		}
		t.Logf("vocab %s first: %v %v (%v)", level, first["headword"], first["reading"], first["gloss"])

		grammar := call("/reference/grammar", map[string]string{"lang": "ja", "level": level, "limit": "3"})
		gItems := grammar["items"].([]any)
		if len(gItems) != 3 {
			t.Fatalf("grammar %s: %d items, want 3", level, len(gItems))
		}
		g := gItems[0].(map[string]any)
		if g["topicId"] == nil || g["description"] == nil || g["example"] == nil {
			t.Errorf("grammar %s: incomplete topic %v", level, g)
		}
	}

	kana := call("/reference/scripts", map[string]string{"lang": "ja", "type": "kana", "limit": "200"})
	if n := len(kana["items"].([]any)); n != 200 {
		t.Fatalf("kana page: %d items, want 200", n)
	}
	firstKana := kana["items"].([]any)[0].(map[string]any)
	if firstKana["glyph"] != "あ" {
		t.Errorf("first kana = %v, want あ (gojūon order)", firstKana["glyph"])
	}

	for _, level := range []string{"N5", "N4", "N2", "N1"} {
		kanji := call("/reference/scripts", map[string]string{"lang": "ja", "type": "kanji", "level": level, "limit": "3"})
		items := kanji["items"].([]any)
		if len(items) != 3 {
			t.Fatalf("kanji %s: %d items, want 3", level, len(items))
		}
		k := items[0].(map[string]any)
		if k["strokeDataRef"] == nil || k["readings"] == nil || k["strokeCount"] == nil {
			t.Errorf("kanji %s: incomplete %v", level, k)
		}
		t.Logf("kanji %s first: %v strokes=%v ref=%v", level, k["glyph"], k["strokeCount"], k["strokeDataRef"])
	}

	var cursor string
	seen := map[string]bool{}
	pages := 0
	for {
		params := map[string]string{"lang": "ja", "level": "N5", "limit": "200"}
		if cursor != "" {
			params["cursor"] = cursor
		}
		body := call("/reference/vocab", params)
		for _, item := range body["items"].([]any) {
			hw := fmt.Sprint(item.(map[string]any)["headword"], item.(map[string]any)["reading"])
			seen[hw] = true
		}
		pages++
		next, _ := body["nextCursor"].(string)
		if next == "" {
			break
		}
		cursor = next
	}
	t.Logf("paged through %d pages, %d distinct N5 entries", pages, len(seen))
	if len(seen) < 700 {
		t.Errorf("N5 distinct entries = %d, want >= 700", len(seen))
	}

	for _, want := range []string{"学校がっこう", "犬いぬ", "食べるたべる"} {
		if !seen[want] {
			t.Errorf("core N5 word %s missing", want)
		}
	}
}
