package webpage

import (
	"strings"
)

// robots holds the Allow / Disallow rules of the robots.txt group that applies to us: the group
// naming our User-Agent's product token, else the "*" group. Longest matching rule wins; on a tie
// Allow wins. "$" anchors the end of a rule and "*" matches any run of characters.
type robots struct {
	rules []robotsRule
}

type robotsRule struct {
	allow bool
	path  string
}

func (r *robots) allowed(path string) bool {
	best, allow := -1, true
	for _, rule := range r.rules {
		if rule.path == "" {
			continue // "Disallow:" with no path allows everything
		}
		if robotsMatch(rule.path, path) && (len(rule.path) > best || (len(rule.path) == best && rule.allow)) {
			best, allow = len(rule.path), rule.allow
		}
	}
	return allow
}

func robotsMatch(pattern, path string) bool {
	anchored := strings.HasSuffix(pattern, "$")
	pattern = strings.TrimSuffix(pattern, "$")
	parts := strings.Split(pattern, "*")
	if !strings.HasPrefix(path, parts[0]) {
		return false
	}
	rest := path[len(parts[0]):]
	for _, p := range parts[1:] {
		i := strings.Index(rest, p)
		if i < 0 {
			return false
		}
		rest = rest[i+len(p):]
	}
	return !anchored || rest == "" || strings.HasSuffix(pattern, "*")
}

func parseRobots(body, userAgent string) *robots {
	token := strings.ToLower(strings.SplitN(userAgent, "/", 2)[0])
	groups := map[string][]robotsRule{}
	var agents []string
	inRules := false
	for _, line := range strings.Split(body, "\n") {
		if i := strings.Index(line, "#"); i >= 0 {
			line = line[:i]
		}
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		k, v = strings.ToLower(strings.TrimSpace(k)), strings.TrimSpace(v)
		switch k {
		case "user-agent":
			if inRules {
				agents, inRules = nil, false
			}
			agents = append(agents, strings.ToLower(v))
		case "allow", "disallow":
			inRules = true
			for _, a := range agents {
				groups[a] = append(groups[a], robotsRule{allow: k == "allow", path: v})
			}
		}
	}
	if rules, ok := groups[token]; ok && token != "" {
		return &robots{rules: rules}
	}
	return &robots{rules: groups["*"]}
}
