package services

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_staticAllowList_Allowed(t *testing.T) {
	type args struct {
		key string
	}
	tests := []struct {
		name    string
		allowed []string
		args    args
		want    bool
	}{
		{
			name:    "nil static allowlist",
			allowed: nil,
			args: args{
				key: "test-key",
			},
			want: false,
		},
		{
			name:    "empty static allowlist",
			allowed: []string{},
			args: args{
				key: "test-key",
			},
			want: false,
		},
		{
			name:    "allowed value in allowlist",
			allowed: []string{"allowed"},
			args: args{
				key: "allowed",
			},
			want: true,
		},
		{
			name:    "unallowed value in allowlist",
			allowed: []string{"allowed"},
			args: args{
				key: "unallowed",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := require.New(t)

			l := NewStaticAllowlist(tt.allowed)
			req.Equal(tt.want, l.Allowed(tt.args.key))
		})
	}
}
