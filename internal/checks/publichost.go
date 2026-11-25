package checks

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/EMOBase/emobase-cli/internal/workspace"
)

// checkPublicHost keeps PUBLIC_HOST equal to the hostname of PUBLIC_SERVER.
// The species nginx serves only PUBLIC_HOST, so a stale value rejects every
// request for the site, and a wrong value would serve a name the deployment
// does not own.
func checkPublicHost(r *Report, st *workspace.Stack) {
	server, _ := st.Get("PUBLIC_SERVER")
	server = strings.TrimSpace(server)
	if server == "" {
		return // required.PUBLIC_SERVER reports the missing value
	}
	u, err := url.Parse(server)
	if err != nil || u.Hostname() == "" {
		r.add(Finding{
			Check:    "public_server.invalid",
			Scope:    scopeOf(st),
			Severity: Error,
			Fix:      FixNone,
			Message:  "PUBLIC_SERVER has no usable hostname",
			Hint:     "use a full URL such as https://tcas.example.org",
		})
		return
	}

	want := u.Hostname()
	got, ok := st.Get("PUBLIC_HOST")
	got = strings.TrimSpace(got)

	switch {
	case !ok || got == "":
		r.add(Finding{
			Check:    "public_host.missing",
			Scope:    scopeOf(st),
			Severity: Error,
			Fix:      FixSafe,
			Message:  "PUBLIC_HOST is missing",
			Hint:     fmt.Sprintf("`emobase doctor --fix` sets it to %q", want),
			apply:    setter(st, "PUBLIC_HOST", want, false),
		})
	case !strings.EqualFold(got, want):
		r.add(Finding{
			Check:    "public_host.mismatch",
			Scope:    scopeOf(st),
			Severity: Error,
			Fix:      FixSafe,
			Message:  fmt.Sprintf("PUBLIC_HOST is %q but PUBLIC_SERVER uses %q", got, want),
			Hint:     fmt.Sprintf("`emobase doctor --fix` sets it to %q; the species nginx serves only this name", want),
			apply:    setter(st, "PUBLIC_HOST", want, false),
		})
	}
}
