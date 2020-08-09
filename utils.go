package main

import "strings"

func tokens(strs ...string) (result []string) {
	for _, s := range strs {
		for _, t := range strings.Split(s, ",") {
			if t = strings.TrimSpace(t); t != "" {
				result = append(result, t)
			}
		}
	}
	return
}

func downcase(strs ...string) (result []string) {
	for _, s := range strs {
		result = append(result, strings.ToLower(s))
	}
	return
}

func uniq(strs ...string) (result []string) {
	seen := map[string]bool{}
	for _, s := range strs {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	return
}
