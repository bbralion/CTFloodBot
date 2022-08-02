package services

// Allowlist specifies values that are explicitly allowed to be used
type Allowlist interface {
	Allowed(key string) bool
}

type staticAllowList struct {
	allowed map[string]struct{}
}

func (l *staticAllowList) Allowed(key string) bool {
	_, ok := l.allowed[key]
	return ok
}

// NewStaticAllowlist returns an allowlist that allows only the values specified
func NewStaticAllowlist(allowed []string) Allowlist {
	m := make(map[string]struct{}, len(allowed))
	for _, s := range allowed {
		m[s] = struct{}{}
	}
	return &staticAllowList{m}
}
