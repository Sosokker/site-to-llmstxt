package crawler

import "strings"

func allowedDomains(host string) []string {
	host = strings.TrimSpace(host)
	if host == "" {
		return nil
	}

	domains := map[string]struct{}{
		host: {},
	}

	if strings.HasPrefix(host, "www.") {
		domains[strings.TrimPrefix(host, "www.")] = struct{}{}
	} else {
		domains["www."+host] = struct{}{}
	}

	list := make([]string, 0, len(domains))
	for d := range domains {
		list = append(list, d)
	}
	return list
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
