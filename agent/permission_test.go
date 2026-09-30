package agent

import "testing"

func TestPickAllowOption(t *testing.T) {
	tests := []struct {
		name    string
		options []permissionOption
		want    string
	}{
		{
			name: "official claude-agent-acp kinds",
			options: []permissionOption{
				{OptionID: "allow_always", Kind: "allow_always"},
				{OptionID: "allow_once", Kind: "allow_once"},
				{OptionID: "reject_once", Kind: "reject_once"},
			},
			want: "allow_once",
		},
		{
			name: "legacy zed adapter ids",
			options: []permissionOption{
				{OptionID: "allow_always", Kind: "allow_always"},
				{OptionID: "allow", Kind: "allow_once"},
				{OptionID: "reject", Kind: "reject_once"},
			},
			want: "allow",
		},
		{
			name:    "only allow_always offered",
			options: []permissionOption{{OptionID: "yes-forever", Kind: "allow_always"}, {OptionID: "no", Kind: "reject_once"}},
			want:    "yes-forever",
		},
		{
			name:    "no allow option falls back",
			options: []permissionOption{{OptionID: "no", Kind: "reject_once"}},
			want:    "allow",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := pickAllowOption(tt.options); got != tt.want {
				t.Errorf("pickAllowOption() = %q, want %q", got, tt.want)
			}
		})
	}
}
