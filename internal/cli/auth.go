package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/marian2js/ocpm/internal/auth"
	"github.com/spf13/cobra"
)

func newAuthCommand(deps Dependencies) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "auth",
		Short:   "Authenticate ocpm with the hosted service",
		GroupID: "support",
	}

	cmd.AddCommand(
		newAuthLoginCommand(deps),
		newAuthStatusCommand(deps),
		newAuthLogoutCommand(deps),
	)
	return cmd
}

func newAuthLoginCommand(deps Dependencies) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Start the device authorization flow",
		RunE: func(cmd *cobra.Command, args []string) error {
			started, err := deps.Auth.StartLogin(cmd.Context())
			if err != nil {
				return err
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "code\t%s\n", started.UserCode)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "verify\t%s\n", started.VerificationURIComplete)
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "waiting\tapprove the request in your browser")
			if !started.BrowserOpened {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "browser\topen the verification URL manually")
			}

			result, err := deps.Auth.WaitForLogin(cmd.Context(), started)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "signed-in\t%s\n", result.User.Email)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "plan\t%s\n", result.Plan)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "registry\t%s\n", result.RegistryURL)
			return nil
		},
	}
	return cmd
}

func newAuthStatusCommand(deps Dependencies) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show the current CLI authentication status",
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := deps.Auth.Status(cmd.Context())
			if err != nil {
				return err
			}

			switch {
			case result.SignedIn:
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "status\tsigned-in")
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "source\t%s\n", result.TokenSource)
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "email\t%s\n", result.User.Email)
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "plan\t%s\n", result.Plan)
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "registry\t%s\n", result.RegistryURL)
			case result.InvalidToken && result.TokenSource == auth.TokenSourceStored:
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "status\tinvalid")
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "message\tstored token is no longer valid")
			case result.InvalidToken && result.TokenSource == auth.TokenSourceEnv:
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "status\tinvalid")
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "message\tthe OCPM_TOKEN value is not valid")
			default:
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "status\tsigned-out")
			}
			return nil
		},
	}
	return cmd
}

func newAuthLogoutCommand(deps Dependencies) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "logout",
		Short: "Revoke the stored CLI token and clear local auth state",
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := deps.Auth.Logout(cmd.Context())
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}

			switch {
			case result.RemoteRevoked:
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "status\tlogged-out")
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "remote\trevoked")
			case result.ClearedSession:
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "status\tlogged-out")
			default:
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "status\tsigned-out")
			}
			return nil
		},
	}
	return cmd
}
