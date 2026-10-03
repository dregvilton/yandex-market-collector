package browser

import "regexp"

var jsonChallenge = regexp.MustCompile(`(?i)"(?:challenge|captcha)(?:Required|_required)?"\s*:\s*(?:true|"required")|"redirect(?:Url|_url)?"\s*:\s*"[^"]*/showcaptcha`)

func hardChallenge(body []byte, sourceURL string) bool {
	return ClassifyChallenge(body, sourceURL).Class == ChallengeHard || jsonChallenge.Match(body)
}
