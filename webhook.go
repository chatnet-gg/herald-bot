// Copyright (c) 2026 ChatNet (sh). MIT licensed, see LICENSE.

package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// maxBodyBytes caps webhook payload size -- GitHub's own delivery size
// limit is 25MB, but nothing this bot announces needs anywhere near
// that; capped low to keep a misbehaving/malicious sender from tying up
// memory reading the body before the signature check even runs.
const maxBodyBytes = 1 << 20 // 1MB

func (b *Bot) handleWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes+1))
	if err != nil {
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}
	if len(body) > maxBodyBytes {
		http.Error(w, "payload too large", http.StatusRequestEntityTooLarge)
		return
	}

	if !validSignature(b.cfg.WebhookSecret, r.Header.Get("X-Hub-Signature-256"), body) {
		logf("webhook: invalid signature from %s", r.RemoteAddr)
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}

	event := r.Header.Get("X-GitHub-Event")
	line, ok := formatEvent(event, body)
	if !ok {
		// Not an error -- most event types this bot doesn't care about
		// (or the "ping" GitHub sends when a webhook is first
		// configured) still get a 200 so GitHub doesn't mark the
		// delivery as failed and keep retrying it.
		w.WriteHeader(http.StatusOK)
		return
	}

	var meta struct {
		Repository struct {
			FullName string `json:"full_name"`
		} `json:"repository"`
	}
	_ = json.Unmarshal(body, &meta)
	b.announce(meta.Repository.FullName, line)
	w.WriteHeader(http.StatusOK)
}

// validSignature checks GitHub's X-Hub-Signature-256 header
// ("sha256=<hex>") against an HMAC-SHA256 of the raw body computed with
// the shared secret -- hmac.Equal for the actual comparison, not ==,
// so this doesn't leak timing information about how much of the
// signature matched.
func validSignature(secret, header string, body []byte) bool {
	const prefix = "sha256="
	if !strings.HasPrefix(header, prefix) {
		return false
	}
	got, err := hex.DecodeString(strings.TrimPrefix(header, prefix))
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	want := mac.Sum(nil)
	return hmac.Equal(got, want)
}

type ghRepo struct {
	FullName string `json:"full_name"`
}

type ghUser struct {
	Login string `json:"login"`
}

type ghPushEvent struct {
	Ref     string `json:"ref"`
	Deleted bool   `json:"deleted"`
	Forced  bool   `json:"forced"`
	Pusher  struct {
		Name string `json:"name"`
	} `json:"pusher"`
	Repository ghRepo `json:"repository"`
	Commits    []struct {
		Message string `json:"message"`
		URL     string `json:"url"`
	} `json:"commits"`
	Compare string `json:"compare"`
}

type ghPullRequestEvent struct {
	Action      string `json:"action"`
	Number      int    `json:"number"`
	Repository  ghRepo `json:"repository"`
	PullRequest struct {
		Title   string `json:"title"`
		HTMLURL string `json:"html_url"`
		Merged  bool   `json:"merged"`
		User    ghUser `json:"user"`
	} `json:"pull_request"`
}

type ghIssuesEvent struct {
	Action     string `json:"action"`
	Repository ghRepo `json:"repository"`
	Issue      struct {
		Number  int    `json:"number"`
		Title   string `json:"title"`
		HTMLURL string `json:"html_url"`
		User    ghUser `json:"user"`
	} `json:"issue"`
}

type ghReleaseEvent struct {
	Action     string `json:"action"`
	Repository ghRepo `json:"repository"`
	Release    struct {
		TagName string `json:"tag_name"`
		Name    string `json:"name"`
		HTMLURL string `json:"html_url"`
		Author  ghUser `json:"author"`
	} `json:"release"`
}

// shortRepo strips the "chatnet-gg/" org prefix when present -- every
// repo this bot will ever see webhooks for lives in that one org (see
// this file's header comment: which repos announce is decided by which
// ones have a webhook configured, not anything this bot filters), so
// the prefix is redundant noise on every single line.
func shortRepo(fullName string) string {
	return strings.TrimPrefix(fullName, "chatnet-gg/")
}

// formatEvent turns one GitHub webhook delivery into a single IRC
// announcement line, or (false) if this event type/action isn't one
// worth announcing (e.g. "ping", a pull_request "labeled" action, or
// any event type this bot doesn't otherwise recognize).
func formatEvent(eventType string, body []byte) (string, bool) {
	switch eventType {
	case "push":
		var ev ghPushEvent
		if err := json.Unmarshal(body, &ev); err != nil {
			return "", false
		}
		if ev.Deleted {
			branch := strings.TrimPrefix(ev.Ref, "refs/heads/")
			return fmt.Sprintf("[%s] %s deleted branch %s", shortRepo(ev.Repository.FullName), ev.Pusher.Name, branch), true
		}
		if len(ev.Commits) == 0 {
			return "", false
		}
		branch := strings.TrimPrefix(ev.Ref, "refs/heads/")
		n := len(ev.Commits)
		commitWord := "commit"
		if n != 1 {
			commitWord = "commits"
		}
		forcedNote := ""
		if ev.Forced {
			forcedNote = " (force-push)"
		}
		last := ev.Commits[n-1].Message
		if idx := strings.IndexByte(last, '\n'); idx != -1 {
			last = last[:idx]
		}
		return fmt.Sprintf("[%s] %s pushed %d %s to %s%s: %s -- %s",
			shortRepo(ev.Repository.FullName), ev.Pusher.Name, n, commitWord, branch, forcedNote, last, ev.Compare), true

	case "pull_request":
		var ev ghPullRequestEvent
		if err := json.Unmarshal(body, &ev); err != nil {
			return "", false
		}
		action := ev.Action
		if ev.Action == "closed" && ev.PullRequest.Merged {
			action = "merged"
		}
		switch ev.Action {
		case "opened", "closed", "reopened":
			return fmt.Sprintf("[%s] %s %s pull request #%d: %s -- %s",
				shortRepo(ev.Repository.FullName), ev.PullRequest.User.Login, action, ev.Number, ev.PullRequest.Title, ev.PullRequest.HTMLURL), true
		}
		return "", false

	case "issues":
		var ev ghIssuesEvent
		if err := json.Unmarshal(body, &ev); err != nil {
			return "", false
		}
		switch ev.Action {
		case "opened", "closed", "reopened":
			return fmt.Sprintf("[%s] %s %s issue #%d: %s -- %s",
				shortRepo(ev.Repository.FullName), ev.Issue.User.Login, ev.Action, ev.Issue.Number, ev.Issue.Title, ev.Issue.HTMLURL), true
		}
		return "", false

	case "release":
		var ev ghReleaseEvent
		if err := json.Unmarshal(body, &ev); err != nil {
			return "", false
		}
		if ev.Action != "published" {
			return "", false
		}
		name := ev.Release.Name
		if name == "" {
			name = ev.Release.TagName
		}
		return fmt.Sprintf("[%s] %s published release %s: %s -- %s",
			shortRepo(ev.Repository.FullName), ev.Release.Author.Login, ev.Release.TagName, name, ev.Release.HTMLURL), true

	default:
		return "", false
	}
}
