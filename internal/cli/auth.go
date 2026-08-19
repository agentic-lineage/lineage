package cli

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/lineage-dev/lineage/internal/auth"
)

// runLogin walks a publisher through GitHub's OAuth device flow and stores
// the resulting token locally, so `lineage package publish` can identify
// them without a token anyone had to hand-mint - see docs/decisions/
// 0012-v1-distribution-contract-and-receiver-activation.md, decision 6b.
func runLogin(home string, stdout, stderr io.Writer) error {
	dc, err := auth.RequestDeviceCode()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return err
	}

	fmt.Fprintf(stdout, "To authorize Lineage to publish as you, go to:\n\n  %s\n\nand enter this code: %s\n\nWaiting for approval...\n", dc.VerificationURI, dc.UserCode)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(dc.ExpiresIn)*time.Second)
	defer cancel()

	token, err := auth.PollForToken(ctx, dc)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return err
	}

	login, err := auth.WhoAmI(token)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return err
	}

	if err := auth.SaveToken(home, token); err != nil {
		fmt.Fprintln(stderr, err)
		return err
	}

	fmt.Fprintf(stdout, "logged in as %s\n", login)
	return nil
}

func runLogout(home string, stdout, stderr io.Writer) error {
	if err := auth.DeleteToken(home); err != nil {
		fmt.Fprintln(stderr, err)
		return err
	}
	fmt.Fprintln(stdout, "logged out")
	return nil
}

// runWhoAmI prints the GitHub login that would be used to publish right
// now - useful to confirm `lineage login` worked, or to check which
// identity LINEAGE_PUBLISH_TOKEN resolves to before publishing.
func runWhoAmI(home string, stdout, stderr io.Writer) error {
	token, err := resolveGitHubToken(home)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return err
	}
	if token == "" {
		err := fmt.Errorf("not logged in; run `lineage login`, or set LINEAGE_PUBLISH_TOKEN for non-interactive use")
		fmt.Fprintln(stderr, err)
		return err
	}

	login, err := auth.WhoAmI(token)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return err
	}
	fmt.Fprintf(stdout, "%s\n", login)
	return nil
}
