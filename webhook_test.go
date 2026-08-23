// Copyright (c) 2026 ChatNet (sh). MIT licensed, see LICENSE.

package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func TestValidSignature(t *testing.T) {
	body := []byte(`{"hello":"world"}`)
	sig := sign("s3cret", body)

	if !validSignature("s3cret", sig, body) {
		t.Fatal("correct signature rejected")
	}
	if validSignature("wrong", sig, body) {
		t.Fatal("signature accepted with wrong secret")
	}
	if validSignature("s3cret", sig, []byte(`{"hello":"tampered"}`)) {
		t.Fatal("signature accepted for tampered body")
	}
	if validSignature("s3cret", "not-even-hex", body) {
		t.Fatal("malformed header accepted")
	}
	if validSignature("s3cret", "", body) {
		t.Fatal("empty header accepted")
	}
}

func TestFormatEventPush(t *testing.T) {
	body := []byte(`{
		"ref": "refs/heads/main",
		"pusher": {"name": "sh"},
		"repository": {"full_name": "chatnet-gg/cerberus-bot"},
		"commits": [{"message": "fix regex\n\nlonger body here"}],
		"compare": "https://github.com/chatnet-gg/cerberus-bot/compare/abc...def"
	}`)
	line, ok := formatEvent("push", body)
	if !ok {
		t.Fatal("expected a formatted line")
	}
	want := "[cerberus-bot] sh pushed 1 commit to main: fix regex -- https://github.com/chatnet-gg/cerberus-bot/compare/abc...def"
	if line != want {
		t.Fatalf("got %q, want %q", line, want)
	}
}

func TestFormatEventPushEmptyCommitsIgnored(t *testing.T) {
	// A push with no commits (e.g. a tag push with nothing new) isn't
	// worth announcing.
	body := []byte(`{"ref":"refs/heads/main","repository":{"full_name":"chatnet-gg/x"},"commits":[]}`)
	if _, ok := formatEvent("push", body); ok {
		t.Fatal("expected no announcement for an empty commit list")
	}
}

func TestFormatEventPullRequest(t *testing.T) {
	body := []byte(`{
		"action": "opened",
		"number": 5,
		"repository": {"full_name": "chatnet-gg/znc-bot"},
		"pull_request": {"title": "Add retry logic", "html_url": "https://github.com/chatnet-gg/znc-bot/pull/5", "user": {"login": "sh"}}
	}`)
	line, ok := formatEvent("pull_request", body)
	if !ok {
		t.Fatal("expected a formatted line")
	}
	want := "[znc-bot] sh opened pull request #5: Add retry logic -- https://github.com/chatnet-gg/znc-bot/pull/5"
	if line != want {
		t.Fatalf("got %q, want %q", line, want)
	}
}

func TestFormatEventPullRequestMerged(t *testing.T) {
	body := []byte(`{
		"action": "closed",
		"number": 5,
		"repository": {"full_name": "chatnet-gg/znc-bot"},
		"pull_request": {"title": "Add retry logic", "html_url": "https://github.com/chatnet-gg/znc-bot/pull/5", "merged": true, "user": {"login": "sh"}}
	}`)
	line, ok := formatEvent("pull_request", body)
	if !ok {
		t.Fatal("expected a formatted line")
	}
	if line != "[znc-bot] sh merged pull request #5: Add retry logic -- https://github.com/chatnet-gg/znc-bot/pull/5" {
		t.Fatalf("got %q", line)
	}
}

func TestFormatEventPullRequestIgnoredAction(t *testing.T) {
	body := []byte(`{"action":"labeled","number":5,"repository":{"full_name":"chatnet-gg/x"},"pull_request":{}}`)
	if _, ok := formatEvent("pull_request", body); ok {
		t.Fatal("expected \"labeled\" action to be ignored")
	}
}

func TestFormatEventIssues(t *testing.T) {
	body := []byte(`{
		"action": "opened",
		"repository": {"full_name": "chatnet-gg/cerberus-bot"},
		"issue": {"number": 3, "title": "KCHECK is slow", "html_url": "https://github.com/chatnet-gg/cerberus-bot/issues/3", "user": {"login": "w0od"}}
	}`)
	line, ok := formatEvent("issues", body)
	if !ok {
		t.Fatal("expected a formatted line")
	}
	want := "[cerberus-bot] w0od opened issue #3: KCHECK is slow -- https://github.com/chatnet-gg/cerberus-bot/issues/3"
	if line != want {
		t.Fatalf("got %q, want %q", line, want)
	}
}

func TestFormatEventRelease(t *testing.T) {
	body := []byte(`{
		"action": "published",
		"repository": {"full_name": "chatnet-gg/cnircd"},
		"release": {"tag_name": "v1.1", "name": "v1.1", "html_url": "https://github.com/chatnet-gg/cnircd/releases/tag/v1.1", "author": {"login": "sh"}}
	}`)
	line, ok := formatEvent("release", body)
	if !ok {
		t.Fatal("expected a formatted line")
	}
	want := "[cnircd] sh published release v1.1: v1.1 -- https://github.com/chatnet-gg/cnircd/releases/tag/v1.1"
	if line != want {
		t.Fatalf("got %q, want %q", line, want)
	}
}

func TestFormatEventUnrecognizedType(t *testing.T) {
	if _, ok := formatEvent("ping", []byte(`{}`)); ok {
		t.Fatal("expected ping event to be ignored")
	}
	if _, ok := formatEvent("star", []byte(`{}`)); ok {
		t.Fatal("expected an unrecognized event type to be ignored")
	}
}

func TestShortRepo(t *testing.T) {
	if got := shortRepo("chatnet-gg/cerberus-bot"); got != "cerberus-bot" {
		t.Fatalf("got %q", got)
	}
	if got := shortRepo("someone-else/other-repo"); got != "someone-else/other-repo" {
		t.Fatalf("got %q, expected non-chatnet-gg repos left untouched", got)
	}
}
