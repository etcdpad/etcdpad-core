package httphandle

import "testing"

func TestUrlParse(t *testing.T) {
	u := "etcd://root:tcFlight9487@172.18.64.41:2379,172.18.178.21:2379,172.18.178.22:2379/"
	uinfo, err := extractURL(u)
	if err != nil {
		t.Fatal(err)
	}

	if uinfo.user != "root" || uinfo.pass != "tcFlight9487" {
		t.Fatalf("user or pass expected root, tcFlight9487, now is %s, %s", uinfo.user, uinfo.pass)
	}

	t.Fatal(uinfo)
}
