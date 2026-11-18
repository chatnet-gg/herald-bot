package main

import (
	"reflect"
	"testing"
)

func TestParseRepoChannels(t *testing.T) {
	got := parseRepoChannels("Owner/Repo=#Dev; owner/other=#a,#b ;broken;=#x;repo/none=nochan")
	want := map[string][]string{
		"owner/repo":  {"#Dev"},
		"owner/other": {"#a", "#b"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if n := len(parseRepoChannels("")); n != 0 {
		t.Fatalf("empty env should give no extras, got %d", n)
	}
}

func TestChannelsFor(t *testing.T) {
	b := &Bot{cfg: Config{
		Channel:      "#Main",
		RepoChannels: parseRepoChannels("owner/repo=#Dev,#main"),
	}}
	if got := b.channelsFor("Owner/Repo"); !reflect.DeepEqual(got, []string{"#Main", "#Dev"}) {
		t.Fatalf("mapped repo: %v", got)
	}
	if got := b.channelsFor("someone/else"); !reflect.DeepEqual(got, []string{"#Main"}) {
		t.Fatalf("an unmapped repo should only go to the main channel: %v", got)
	}
	if got := b.allChannels(); !reflect.DeepEqual(got, []string{"#Main", "#Dev"}) {
		t.Fatalf("allChannels: %v", got)
	}
}
