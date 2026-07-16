package config

import "testing"

func TestUpsertAndPrimary(t *testing.T) {
	c := &Config{}
	c.Upsert(Host{Name: "a.com", URL: "https://a.com"})
	if p := c.Primary(); p == nil || p.Name != "a.com" {
		t.Fatal("first host should become primary")
	}

	c.Upsert(Host{Name: "b.com", URL: "https://b.com"})
	if p := c.Primary(); p.Name != "a.com" {
		t.Errorf("primary changed unexpectedly to %s", p.Name)
	}

	c.Upsert(Host{Name: "b.com", URL: "https://b.com", Primary: true})
	if p := c.Primary(); p.Name != "b.com" {
		t.Errorf("primary = %s, want b.com", p.Name)
	}
	// exactly one primary
	n := 0
	for i := range c.Hosts {
		if c.Hosts[i].Primary {
			n++
		}
	}
	if n != 1 {
		t.Errorf("expected exactly one primary, got %d", n)
	}
}

func TestRemovePromotes(t *testing.T) {
	c := &Config{}
	c.Upsert(Host{Name: "a.com"})
	c.Upsert(Host{Name: "b.com"})
	if !c.Remove("a.com") { // a was primary
		t.Fatal("remove failed")
	}
	if p := c.Primary(); p == nil || p.Name != "b.com" {
		t.Error("b.com should have been promoted to primary")
	}
	if c.Remove("nope") {
		t.Error("removing unknown host should return false")
	}
}

func TestHooks(t *testing.T) {
	c := &Config{}
	c.AddHook(Hook{Dir: "/a", Mode: "permanent"})
	c.AddHook(Hook{Dir: "/b", Mode: "temp", TTL: "30d"})
	if len(c.Hooks) != 2 {
		t.Fatalf("expected 2 hooks, got %d", len(c.Hooks))
	}

	// AddHook replaces by directory rather than duplicating.
	c.AddHook(Hook{Dir: "/a", Mode: "temp", TTL: "1d"})
	if len(c.Hooks) != 2 {
		t.Errorf("AddHook duplicated instead of replacing: %d hooks", len(c.Hooks))
	}
	if h := c.FindHook("/a"); h == nil || h.Mode != "temp" || h.TTL != "1d" {
		t.Errorf("FindHook returned stale hook: %+v", h)
	}

	if !c.RemoveHook("/a") {
		t.Error("RemoveHook(/a) should succeed")
	}
	if c.FindHook("/a") != nil {
		t.Error("/a still present after removal")
	}
	if c.RemoveHook("/missing") {
		t.Error("RemoveHook of unknown dir should return false")
	}
}

func TestNormalizeURL(t *testing.T) {
	cases := []struct {
		in       string
		wantURL  string
		wantName string
	}{
		{"files.example.com", "https://files.example.com", "files.example.com"},
		{"http://localhost:8099", "http://localhost:8099", "localhost"},
		{"https://x.io/", "https://x.io", "x.io"},
	}
	for _, c := range cases {
		url, name, err := NormalizeURL(c.in)
		if err != nil {
			t.Errorf("NormalizeURL(%q): %v", c.in, err)
			continue
		}
		if url != c.wantURL || name != c.wantName {
			t.Errorf("NormalizeURL(%q) = (%q, %q), want (%q, %q)", c.in, url, name, c.wantURL, c.wantName)
		}
	}
}
