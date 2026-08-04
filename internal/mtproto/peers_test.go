package mtproto

import "testing"

func TestClassifyID(t *testing.T) {
	tests := []struct {
		name     string
		id       int64
		wantType string
		wantID   int64
	}{
		{name: "positive user", id: 12345, wantType: "user", wantID: 12345},
		{name: "telegram user", id: 777000, wantType: "user", wantID: 777000},
		{name: "legacy chat", id: -56789, wantType: "chat", wantID: 56789},
		// Real @telegram channel: raw peer id is -(1000000000000 + 1186985868).
		{name: "channel -100 prefix", id: -1001186985868, wantType: "channel", wantID: 1186985868},
		{name: "channel minimal", id: -1000000000001, wantType: "channel", wantID: 1},
		{name: "channel large", id: -1001800000001, wantType: "channel", wantID: 1800000001},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			typ, gotID := classifyID(tc.id)
			if typ != tc.wantType || gotID != tc.wantID {
				t.Errorf("classifyID(%d) = (%q, %d), want (%q, %d)", tc.id, typ, gotID, tc.wantType, tc.wantID)
			}
			// The mapping must be stable: classifying the result of the channel
			// strip (a positive channel id with an unset sign) must not keep
			// shifting the value, which was the old bug's symptom.
			typ2, gotID2 := classifyID(tc.id)
			if typ2 != tc.wantType || gotID2 != tc.wantID {
				t.Errorf("classifyID(%d) not idempotent: second call = (%q, %d)", tc.id, typ2, gotID2)
			}
		})
	}
}
