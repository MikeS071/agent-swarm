package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/MikeS071/agent-swarm/internal/config"
	"github.com/MikeS071/agent-swarm/internal/feature"
	"github.com/MikeS071/agent-swarm/internal/tracker"
	"github.com/spf13/cobra"
)

var featureCmd = &cobra.Command{
	Use:   "feature",
	Short: "Manage feature lifecycle",
}

var featureAddCmd = &cobra.Command{
	Use:          "add <name>",
	Short:        "Create a feature in draft state",
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := loadFeatureStore()
		if err != nil {
			return err
		}
		prdPath, err := cmd.Flags().GetString("prd")
		if err != nil {
			return err
		}

		f, err := store.Add(args[0])
		if err != nil {
			return err
		}
		if strings.TrimSpace(prdPath) != "" {
			if err := copyFile(prdPath, filepath.Join(store.FeatureDir(args[0]), "prd.md")); err != nil {
				return err
			}
		}

		_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\n", f.Name, f.State)
		return err
	},
}

var featureListCmd = &cobra.Command{
	Use:          "list",
	Short:        "List features and states",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		store, err := loadFeatureStore()
		if err != nil {
			return err
		}

		features, err := store.List()
		if err != nil {
			return err
		}
		for _, f := range features {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\n", f.Name, f.State); err != nil {
				return err
			}
		}
		return nil
	},
}

var featureShowCmd = &cobra.Command{
	Use:          "show <name>",
	Short:        "Show feature metadata",
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := loadFeatureStore()
		if err != nil {
			return err
		}
		asJSON, err := cmd.Flags().GetBool("json")
		if err != nil {
			return err
		}
		f, err := store.Get(args[0])
		if err != nil {
			return err
		}

		if asJSON {
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(f)
		}

		_, err = fmt.Fprintf(cmd.OutOrStdout(), "name: %s\nstate: %s\n", f.Name, f.State)
		return err
	},
}

var featureApprovePRDCmd = &cobra.Command{
	Use:          "approve-prd <name>",
	Short:        "Approve PRD and advance to arch_review",
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := loadFeatureStore()
		if err != nil {
			return err
		}
		by, err := cmd.Flags().GetString("by")
		if err != nil {
			return err
		}
		name := args[0]

		if err := requireFile(filepath.Join(store.FeatureDir(name), "prd.md"), "prd.md"); err != nil {
			return err
		}

		f, err := store.Get(name)
		if err != nil {
			return err
		}
		if f.State == feature.StateDraft {
			if _, err := store.Advance(name, feature.StatePRDReview); err != nil {
				return err
			}
			f, err = store.Get(name)
			if err != nil {
				return err
			}
		}
		if f.State != feature.StatePRDReview {
			return fmt.Errorf("feature %q must be in %q state, got %q", name, feature.StatePRDReview, f.State)
		}

		f, err = store.Advance(name, feature.StateArchReview)
		if err != nil {
			return err
		}
		f.PRDApprovedAt = time.Now().UTC().Format(time.RFC3339)
		f.PRDApprovedBy = approvedBy(by)
		if err := store.Save(f); err != nil {
			return err
		}

		_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\n", f.Name, f.State)
		return err
	},
}

var featureArchReviewCmd = &cobra.Command{
	Use:          "arch-review <name>",
	Short:        "Mark architecture review complete and advance to spec_review",
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := loadFeatureStore()
		if err != nil {
			return err
		}
		name := args[0]

		f, err := store.Get(name)
		if err != nil {
			return err
		}
		if f.State != feature.StateArchReview {
			return fmt.Errorf("feature %q must be in %q state, got %q", name, feature.StateArchReview, f.State)
		}
		if err := requireFile(filepath.Join(store.FeatureDir(name), "arch-review.md"), "arch-review.md"); err != nil {
			return err
		}

		f, err = store.Advance(name, feature.StateSpecReview)
		if err != nil {
			return err
		}
		f.ArchReviewAt = time.Now().UTC().Format(time.RFC3339)
		if err := store.Save(f); err != nil {
			return err
		}

		_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\n", f.Name, f.State)
		return err
	},
}

var featureApproveSpecCmd = &cobra.Command{
	Use:          "approve-spec <name>",
	Short:        "Approve spec and advance to planned",
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := loadFeatureStore()
		if err != nil {
			return err
		}
		by, err := cmd.Flags().GetString("by")
		if err != nil {
			return err
		}
		name := args[0]
		featureDir := store.FeatureDir(name)

		if err := requireFile(filepath.Join(featureDir, "arch-review.md"), "arch-review.md"); err != nil {
			return err
		}
		if err := requireFile(filepath.Join(featureDir, "spec.md"), "spec.md"); err != nil {
			return err
		}

		f, err := store.Get(name)
		if err != nil {
			return err
		}
		if f.State == feature.StateArchReview {
			if _, err := store.Advance(name, feature.StateSpecReview); err != nil {
				return err
			}
			f, err = store.Get(name)
			if err != nil {
				return err
			}
		}
		if f.State != feature.StateSpecReview {
			return fmt.Errorf("feature %q must be in %q state, got %q", name, feature.StateSpecReview, f.State)
		}

		f, err = store.Advance(name, feature.StatePlanned)
		if err != nil {
			return err
		}
		f.SpecApprovedAt = time.Now().UTC().Format(time.RFC3339)
		f.SpecApprovedBy = approvedBy(by)
		if err := store.Save(f); err != nil {
			return err
		}

		_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\n", f.Name, f.State)
		return err
	},
}

var featurePlanCmd = &cobra.Command{
	Use:          "plan <name>",
	Short:        "Advance feature from planned to building",
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := loadFeatureStore()
		if err != nil {
			return err
		}
		name := args[0]

		f, err := store.Get(name)
		if err != nil {
			return err
		}
		if f.State != feature.StatePlanned {
			return fmt.Errorf("feature %q must be in %q state, got %q", name, feature.StatePlanned, f.State)
		}

		f, err = store.Advance(name, feature.StateBuilding)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\n", f.Name, f.State)
		return err
	},
}

var featureCompleteCmd = &cobra.Command{
	Use:          "complete <name>",
	Short:        "Mark feature complete",
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, store, err := loadFeatureStoreWithConfig()
		if err != nil {
			return err
		}
		name := args[0]
		f, err := store.Get(name)
		if err != nil {
			return err
		}
		if f.State != feature.StatePostBuild {
			return fmt.Errorf("feature %q must be in %q state, got %q", name, feature.StatePostBuild, f.State)
		}

		trackerPath := resolveFromConfig(cfgFile, cfg.Project.Tracker)
		tr, err := tracker.Load(trackerPath)
		if err != nil {
			return err
		}

		if err := requireDoneTickets(tr, f.PostBuildTickets, "post_build_tickets"); err != nil {
			return err
		}
		if err := requireDoneTickets(tr, f.FixTickets, "fix_tickets"); err != nil {
			return err
		}

		f, err = store.Advance(name, feature.StateComplete)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\n", f.Name, f.State)
		return err
	},
}

func init() {
	featureAddCmd.Flags().String("prd", "", "path to PRD file to copy into feature directory")
	featureShowCmd.Flags().Bool("json", false, "output feature metadata as JSON")
	featureApprovePRDCmd.Flags().String("by", "", "approver name")
	featureApproveSpecCmd.Flags().String("by", "", "approver name")

	featureCmd.AddCommand(featureAddCmd)
	featureCmd.AddCommand(featureListCmd)
	featureCmd.AddCommand(featureShowCmd)
	featureCmd.AddCommand(featureApprovePRDCmd)
	featureCmd.AddCommand(featureArchReviewCmd)
	featureCmd.AddCommand(featureApproveSpecCmd)
	featureCmd.AddCommand(featurePlanCmd)
	featureCmd.AddCommand(featureCompleteCmd)

	rootCmd.AddCommand(featureCmd)
}

func loadFeatureStore() (*feature.Store, error) {
	_, store, err := loadFeatureStoreWithConfig()
	if err != nil {
		return nil, err
	}
	return store, nil
}

func loadFeatureStoreWithConfig() (*config.Config, *feature.Store, error) {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return nil, nil, err
	}
	root := resolveFromConfig(cfgFile, featureRoot(cfg))
	return cfg, feature.NewStore(root), nil
}

func featureRoot(cfg *config.Config) string {
	path := strings.TrimSpace(cfg.Project.FeaturesDir)
	if path == "" {
		return "swarm/features"
	}
	return path
}

func requireFile(path, name string) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%s does not exist at %s", name, path)
		}
		return fmt.Errorf("stat %s: %w", path, err)
	}
	if info.IsDir() {
		return fmt.Errorf("%s is a directory at %s", name, path)
	}
	return nil
}

func approvedBy(value string) string {
	by := strings.TrimSpace(value)
	if by != "" {
		return by
	}
	by = strings.TrimSpace(os.Getenv("USER"))
	if by != "" {
		return by
	}
	return "unknown"
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("read source file %s: %w", src, err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("mkdir destination dir: %w", err)
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		return fmt.Errorf("write destination file %s: %w", dst, err)
	}
	return nil
}

func requireDoneTickets(tr *tracker.Tracker, ids []string, field string) error {
	for _, id := range ids {
		tk, ok := tr.Get(id)
		if !ok {
			return fmt.Errorf("%s references missing ticket %q", field, id)
		}
		if tk.Status != tracker.StatusDone {
			return fmt.Errorf("%s ticket %q is %q, expected %q", field, id, tk.Status, tracker.StatusDone)
		}
	}
	return nil
}
