package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/justin06lee/caveira/internal/fabricate"
)

func TestWriteMailmapSkeleton(t *testing.T) {
	discovered := []fabricate.DiscoveredIdentity{
		{Identity: fabricate.Identity{Name: "justin06lee", Email: "hi@justin06lee.dev"}, Commits: 131},
		{Identity: fabricate.Identity{Name: "justin06lee", Email: "justin.leehuiyun@gmail.com"}, Commits: 1},
	}
	var buf bytes.Buffer
	writeMailmapSkeleton(&buf, discovered)
	got := buf.String()
	if !strings.Contains(got, "# .mailmap skeleton") {
		t.Fatalf("missing comment header:\n%s", got)
	}
	if !strings.Contains(got, "justin06lee <hi@justin06lee.dev>\n") {
		t.Fatalf("missing first identity line:\n%s", got)
	}
	if !strings.Contains(got, "justin06lee <justin.leehuiyun@gmail.com>\n") {
		t.Fatalf("missing second identity line:\n%s", got)
	}
}

func TestWriteMailmapSkeleton_EmptyHistory(t *testing.T) {
	var buf bytes.Buffer
	writeMailmapSkeleton(&buf, nil)
	if !strings.Contains(buf.String(), "# .mailmap skeleton") {
		t.Fatalf("empty history should still print the comment header:\n%s", buf.String())
	}
}
