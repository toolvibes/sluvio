package haproxy

import "testing"

func TestParseMessages(t *testing.T) {
	out := `[NOTICE]   (1) : haproxy version is 2.9.4
[ALERT]    (1) : config : parsing [/etc/haproxy/haproxy.cfg:42] : 'server x' unknown keyword 'frobnicate'.
[WARNING]  (1) : config : parsing [/etc/haproxy/haproxy.cfg:7] : 'option httplog' not usable with frontend 'tcp'.
[ALERT]    (1) : config : Fatal errors found in configuration.`
	m := parseMessages(out)
	if len(m) != 4 {
		t.Fatalf("got %d messages", len(m))
	}
	if m[1].Level != "alert" || m[1].Line != 42 {
		t.Fatalf("bad alert: %+v", m[1])
	}
	if m[2].Level != "warning" || m[2].Line != 7 {
		t.Fatalf("bad warning: %+v", m[2])
	}
	if m[3].Line != 0 {
		t.Fatalf("summary line has no line number: %+v", m[3])
	}
}

func TestParseStatCSV(t *testing.T) {
	csv := "# pxname,svname,qcur,scur,stot,bin,bout,status,weight,act,bck,type,rate,check_status,lastchg,addr\n" +
		"http,FRONTEND,,3,10,100,200,OPEN,,,,0,1,,,\n" +
		"be,s1,0,1,5,10,20,UP,1,1,0,2,0,L7OK,12,10.0.0.1:80\n" +
		"be,s2,0,0,0,0,0,MAINT,1,1,0,2,0,,30,10.0.0.2:80\n" +
		"be,BACKEND,0,1,5,10,20,UP,2,2,0,1,0,,12,\n"
	s, err := ParseStatCSV(csv)
	if err != nil {
		t.Fatal(err)
	}
	if len(s) != 4 || s[1].Type != "server" || s[1].Status != "UP" || s[1].CheckState != "L7OK" || s[2].Status != "MAINT" || s[0].Type != "frontend" {
		t.Fatalf("unexpected %+v", s)
	}
	if _, err := ParseStatCSV("Unknown command\n"); err == nil {
		t.Fatal("expected error for non-CSV answer")
	}
}

func TestRuntimeRejectsInjection(t *testing.T) {
	r := &Runtime{StatsSocket: "/nonexistent"}
	if _, err := r.Exec("show info; shutdown frontend x"); err == nil {
		t.Fatal("command chaining must be rejected")
	}
	if err := r.SetServerState("be", "s1;x", "maint"); err == nil {
		t.Fatal("invalid server name must be rejected")
	}
	if err := r.SetServerState("be", "s1", "stop"); err == nil {
		t.Fatal("invalid state must be rejected")
	}
}
