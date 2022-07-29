package services

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_staticAuthenticator_Authenticate(t *testing.T) {
	type args struct {
		token string
	}
	tests := []struct {
		name    string
		clients map[string]Client
		args    args
		want    Client
		wantErr bool
	}{
		{
			name:    "nil client map",
			clients: nil,
			args:    args{"faketoken"},
			wantErr: true,
		},
		{
			name:    "empty client map",
			clients: map[string]Client{},
			args:    args{"faketoken"},
			wantErr: true,
		},
		{
			name: "valid token",
			clients: map[string]Client{
				"goodtoken": {Name: "client1"},
			},
			args:    args{"goodtoken"},
			want:    Client{Name: "client1"},
			wantErr: false,
		},
		{
			name: "invalid token",
			clients: map[string]Client{
				"goodtoken": {Name: "client1"},
			},
			args:    args{"badtoken"},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := require.New(t)

			p := NewStaticAuthenticator(tt.clients)
			got, err := p.Authenticate(tt.args.token)
			if tt.wantErr {
				req.Error(err)
			}
			req.Equal(tt.want, got)
		})
	}
}
