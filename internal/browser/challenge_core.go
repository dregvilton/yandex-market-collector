package browser

import (
	"regexp"
	"sort"
	"strings"
)

type ChallengeClass string

const (
	ChallengeHard    ChallengeClass = "HARD_CHALLENGE"
	ChallengeSuspect ChallengeClass = "SUSPECT_CHALLENGE_MARKERS"
	ChallengeNormal  ChallengeClass = "NORMAL"
)

type MarkerEvidence struct {
	Marker, Location, Snippet string
}

type ChallengeDiagnostic struct {
	Class                  ChallengeClass
	Reasons                []string
	Title                  string
	SmartCaptchaCount      int
	CaptchaClassCount      int
	ShowCaptchaCount       int
	MarkerEvidence         []MarkerEvidence
	AllMarkersInScriptLike bool
}

var (
	titleRE         = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	scriptStyleRE   = regexp.MustCompile(`(?is)<(?:script|style)\b[^>]*>.*?</(?:script|style)>`)
	captchaRootRE   = regexp.MustCompile(`(?is)<(?:div|form|main|section)[^>]*(?:id|class)\s*=\s*["'][^"']*(?:smart-captcha|captcha__)[^"']*["']`)
	captchaFormRE   = regexp.MustCompile(`(?is)<form[^>]+(?:action|id|class)\s*=\s*["'][^"']*(?:showcaptcha|captcha)[^"']*["']`)
	productSignalRE = regexp.MustCompile(`(?i)(?:oskuId|itemListElement|/card/|/product--|"price"\s*:|data-zone-data)`)
)

func ClassifyChallenge(body []byte, finalURL string) ChallengeDiagnostic {
	s := string(body)
	lower := strings.ToLower(s)
	u := strings.ToLower(finalURL)
	d := ChallengeDiagnostic{Title: htmlTitle(s)}
	d.SmartCaptchaCount = strings.Count(lower, "smart-captcha")
	d.CaptchaClassCount = strings.Count(lower, "captcha__")
	d.ShowCaptchaCount = strings.Count(lower, "showcaptcha")
	d.MarkerEvidence = markerEvidence(s, []string{"smart-captcha", "captcha__", "showcaptcha"}, 12)
	d.AllMarkersInScriptLike = len(d.MarkerEvidence) > 0
	for _, e := range d.MarkerEvidence {
		if e.Location != "script/style" {
			d.AllMarkersInScriptLike = false
			break
		}
	}

	if strings.Contains(u, "/showcaptcha") {
		d.Class = ChallengeHard
		d.Reasons = append(d.Reasons, "final URL contains /showcaptcha")
		return d
	}
	if strings.Contains(u, "/auth/") || strings.Contains(u, "passport.yandex") {
		d.Class = ChallengeHard
		d.Reasons = append(d.Reasons, "final URL is authentication/interstitial")
		return d
	}
	visible := scriptStyleRE.ReplaceAllString(s, " ")
	visibleLower := strings.ToLower(visible)
	productSignals := len(productSignalRE.FindAllStringIndex(s, -1))
	title := strings.ToLower(d.Title)
	challengeTitle := strings.Contains(title, "captcha") || strings.Contains(title, "подтвердите") || strings.Contains(title, "are you human") || strings.Contains(title, "робот")
	root := captchaRootRE.MatchString(visible) || captchaFormRE.MatchString(visible)
	humanPrompt := strings.Contains(visibleLower, "confirm you are human") || strings.Contains(visibleLower, "подтвердите, что вы не робот") || strings.Contains(visibleLower, "пройдите проверку")
	if (root && productSignals == 0) || (challengeTitle && (root || humanPrompt)) {
		d.Class = ChallengeHard
		if root {
			d.Reasons = append(d.Reasons, "visible captcha root/form replaces content")
		}
		if challengeTitle {
			d.Reasons = append(d.Reasons, "challenge-specific HTML title")
		}
		if humanPrompt {
			d.Reasons = append(d.Reasons, "visible human-verification prompt")
		}
		return d
	}
	if d.SmartCaptchaCount+d.CaptchaClassCount+d.ShowCaptchaCount > 0 {
		d.Class = ChallengeSuspect
		d.Reasons = append(d.Reasons, "captcha marker present without hard interstitial structure")
		if d.AllMarkersInScriptLike {
			d.Reasons = append(d.Reasons, "sampled markers occur only in script/style")
		}
		return d
	}
	d.Class = ChallengeNormal
	d.Reasons = []string{"no challenge evidence"}
	return d
}

func htmlTitle(s string) string {
	m := titleRE.FindStringSubmatch(s)
	if len(m) < 2 {
		return ""
	}
	return strings.TrimSpace(regexp.MustCompile(`\s+`).ReplaceAllString(m[1], " "))
}

func markerEvidence(body string, markers []string, limit int) []MarkerEvidence {
	lower := strings.ToLower(body)
	ranges := scriptStyleRE.FindAllStringIndex(body, -1)
	var out []MarkerEvidence
	for _, marker := range markers {
		from := 0
		for len(out) < limit {
			i := strings.Index(lower[from:], marker)
			if i < 0 {
				break
			}
			i += from
			loc := "visible/html"
			for _, r := range ranges {
				if i >= r[0] && i < r[1] {
					loc = "script/style"
					break
				}
			}
			start, end := i-100, i+len(marker)+100
			if start < 0 {
				start = 0
			}
			if end > len(body) {
				end = len(body)
			}
			snippet := regexp.MustCompile(`\s+`).ReplaceAllString(body[start:end], " ")
			out = append(out, MarkerEvidence{Marker: marker, Location: loc, Snippet: snippet})
			from = i + len(marker)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Marker < out[j].Marker })
	return out
}
