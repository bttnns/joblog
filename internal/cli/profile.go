package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/bttnns/joblog/internal/assets"
	"github.com/bttnns/joblog/internal/store"
	"github.com/spf13/cobra"
)

func init() { addCommand(newProfileCmd) }

// newProfileCmd groups the profile verbs under `jl profile`. The profile is the
// distilled identity (profile.md plus accomplishments.md); the raw documents live
// under `jl resume`. A bare `jl profile` runs show.
func newProfileCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "profile",
		Aliases: []string{"narrative"},
		Short:   "Who you are, built from your base resume (profile.md)",
		Long: "profile.md (who you are plus what you want next) and accomplishments.md hold\n" +
			"your distilled identity. Build them with an agent from your base resume, or fill\n" +
			"them in by hand. A bare 'jl profile' prints profile.md.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runProfileShow(cmd)
		},
	}
	cmd.AddCommand(
		newProfileShowCmd(),
		newProfileBuildCmd(),
		newProfileEditCmd(),
		newProfilePromptCmd(),
	)
	return cmd
}

func newProfileShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Print profile.md",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runProfileShow(cmd)
		},
	}
}

func runProfileShow(cmd *cobra.Command) error {
	s, err := openStore(cmd)
	if err != nil {
		return err
	}
	b, err := os.ReadFile(s.Path("profile.md"))
	if os.IsNotExist(err) {
		info("no profile.md yet; build one with: jl profile build  (or fill it by hand: jl profile edit)")
		return nil
	}
	if err != nil {
		return err
	}
	fmt.Print(string(b))
	if len(b) > 0 && b[len(b)-1] != '\n' {
		fmt.Println()
	}
	return nil
}

func newProfileBuildCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "build [<file>]",
		Short: "Print a build-profile prompt to pipe to an agent (optionally set the base resume first)",
		Long: "Scaffold the profile template files if absent, then print the build-profile\n" +
			"prompt plus your resume.txt on stdout so it can be piped to an agent, e.g.\n" +
			"  jl profile build | claude -p\n" +
			"If <file> is given, it sets the base resume first (same as jl resume set). To\n" +
			"fill the files in yourself instead, run jl profile edit.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, release, err := openStoreForWrite(cmd)
			if err != nil {
				return err
			}
			defer release()
			if err := s.Init(); err != nil {
				return err
			}
			if len(args) == 1 {
				if _, _, err := setBaseResume(s, args[0]); err != nil {
					return err
				}
			}

			prompt := assets.BuildProfilePrompt()
			resume, found, err := readResumeText(s.Path("resume", "resume.txt"))
			if err != nil {
				return err
			}

			// The prompt goes to stdout so it can be piped to an agent; guidance
			// goes to stderr so it never pollutes the pipe.
			fmt.Print(prompt)
			fmt.Print("\nRESUME:\n")
			if found {
				fmt.Print(resume)
			} else {
				fmt.Print("(no resume.txt found; run: jl resume set <file>)\n")
			}
			fmt.Println()
			info("pipe this to your agent (e.g. jl profile build | claude -p), or fill it in yourself with jl profile edit")
			if !found {
				info("note: no resume.txt yet; run 'jl resume set <file>' so the agent has your resume")
			}
			return nil
		},
	}
}

func newProfileEditCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "edit",
		Short: "Open profile.md in your editor (by hand, no AI)",
		Long: "Scaffold profile.md if absent, then open it in $EDITOR (falling back to\n" +
			"$VISUAL, then vi). This is the by-hand path; no agent is involved.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			s, release, err := openStoreForWrite(cmd)
			if err != nil {
				return err
			}
			defer release()
			if err := s.Init(); err != nil {
				return err
			}
			return openInEditor(s, "profile.md")
		},
	}
}

// openInEditor opens a data-dir file in the user's editor. It resolves $EDITOR,
// then $VISUAL, then vi, and connects the child to the current stdio so an
// interactive editor works.
func openInEditor(s *store.Store, rel string) error {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = os.Getenv("VISUAL")
	}
	if editor == "" {
		editor = "vi"
	}
	path := s.Path(filepath.FromSlash(rel))
	c := exec.Command(editor, path)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		return fmt.Errorf("run editor %q: %w", editor, err)
	}
	return nil
}

func newProfilePromptCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "prompt",
		Short: "Emit only the raw build-profile prompt to stdout (the clean pipe contract)",
		Long: "Print only the build-profile instruction block to stdout: no scaffolding, no\n" +
			"resume text, no stderr chatter. The clean pipe contract for agents.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Print(assets.BuildProfilePrompt())
			return nil
		},
	}
}

func readResumeText(path string) (string, bool, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return string(b), true, nil
}
