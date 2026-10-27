package containermanager

import (
	"fmt"
	"testing"
)

func TestSELinuxLevelsAreStableAndApart(t *testing.T) {
	seen := map[string]int{}
	for _, uid := range []int{0, 1, 1000, 1001, 1022, 1023, 1024, 2047, 54321, 523775} {
		level := selinuxLevel(uid)
		var first, second int
		if _, err := fmt.Sscanf(level, "s0:c%d,c%d", &first, &second); err != nil || first >= second || second > 1023 {
			t.Fatalf("uid %d gets level %q", uid, level)
		}
		if other, taken := seen[level]; taken {
			t.Fatalf("uids %d and %d share %s", other, uid, level)
		}
		seen[level] = uid
		if selinuxLevel(uid) != level {
			t.Fatal("a user's level changed between starts")
		}
	}
	labels := newSELinuxLabels(1000)
	if labels.process != "system_u:system_r:container_t:"+labels.level || labels.file != "system_u:object_r:container_file_t:"+labels.level {
		t.Fatalf("labels %+v", labels)
	}
}

func TestSELinuxRelabelKeepsUserAndRole(t *testing.T) {
	for context, want := range map[string]string{
		"unconfined_u:object_r:user_home_t:s0":         "unconfined_u:object_r:container_file_t:s0:c1,c2",
		"system_u:object_r:user_tmp_t:s0:c0.c1023":     "system_u:object_r:container_file_t:s0:c1,c2",
		"unconfined_u:object_r:container_file_t:s0:c7": "unconfined_u:object_r:container_file_t:s0:c1,c2",
		"garbage": "system_u:object_r:container_file_t:s0:c1,c2",
	} {
		if got := withTypeAndLevel(context, "container_file_t", "s0:c1,c2"); got != want {
			t.Errorf("%s became %s, want %s", context, got, want)
		}
	}
}
