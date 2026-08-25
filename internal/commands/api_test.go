package commands

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestAPICallFlagsParsed(t *testing.T) {
	// Verify that the flags are accepted without error at parse time (args parsing only)
	rootCmd := &cobra.Command{Use: "port"}
	RegisterAPI(rootCmd)

	apiCmd, _, _ := rootCmd.Find([]string{"api"})
	if apiCmd == nil {
		t.Fatal("api command not found")
	}

	callCmd, _, _ := apiCmd.Find([]string{"call"})
	if callCmd == nil {
		t.Fatal("call command not found")
	}

	// Parse args without executing RunE
	callCmd.DisableFlagParsing = false
	err := callCmd.ParseFlags([]string{
		"--org", "local",
		"--method", "POST",
		"--data", "{}",
		"--format", "yaml",
		"--unwrap", "blueprints",
	})
	if err != nil {
		t.Fatalf("unexpected error parsing flags: %v", err)
	}

	org, err := callCmd.Flags().GetString("org")
	if err != nil {
		t.Fatalf("could not get --org %v", err)
	}
	if org != "local" {
		t.Errorf("expected 'local', got %q", org)
	}

	method, err := callCmd.Flags().GetString("method")
	if err != nil {
		t.Fatalf("could not get --method %v", err)
	}
	if method != "POST" {
		t.Errorf("expected 'POST', got %q", method)
	}

	data, err := callCmd.Flags().GetString("data")
	if err != nil {
		t.Fatalf("could not get --data %v", err)
	}
	if data != "{}" {
		t.Errorf("expected '{}', got %q", data)
	}

	format, err := callCmd.Flags().GetString("format")
	if err != nil {
		t.Fatalf("could not get --format %v", err)
	}
	if format != "yaml" {
		t.Errorf("expected 'yaml', got %q", format)
	}

	unwrap, err := callCmd.Flags().GetString("unwrap")
	if err != nil {
		t.Fatalf("could not get --unwrap %v", err)
	}
	if unwrap != "blueprints" {
		t.Errorf("expected 'blueprints', got %q", unwrap)
	}
}

func TestPageSubcommandsFlagsParsed(t *testing.T) {
	rootCmd := &cobra.Command{Use: "port"}
	RegisterAPI(rootCmd)

	apiCmd, _, _ := rootCmd.Find([]string{"api"})
	pagesCmd, _, _ := apiCmd.Find([]string{"pages"})
	if pagesCmd == nil {
		t.Fatal("pages command not found")
	}

	listCmd, _, _ := pagesCmd.Find([]string{"list"})
	if listCmd == nil {
		t.Fatal("pages list command not found")
	}

	createCmd, _, _ := pagesCmd.Find([]string{"create"})
	if createCmd == nil {
		t.Fatal("pages create command not found")
	}
	err := createCmd.ParseFlags([]string{"--data", "page.json"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	dataFile, _ := createCmd.Flags().GetString("data")
	if dataFile != "page.json" {
		t.Errorf("expected 'page.json', got %q", dataFile)
	}

	updateCmd, _, _ := pagesCmd.Find([]string{"update"})
	if updateCmd == nil {
		t.Fatal("pages update command not found")
	}
}

func TestTeamSubcommandsFlagsParsed(t *testing.T) {
	rootCmd := &cobra.Command{Use: "port"}
	RegisterAPI(rootCmd)

	apiCmd, _, _ := rootCmd.Find([]string{"api"})
	teamsCmd, _, _ := apiCmd.Find([]string{"teams"})
	if teamsCmd == nil {
		t.Fatal("teams command not found")
	}

	for _, sub := range []string{"list", "create", "update", "delete"} {
		subCmd, _, _ := teamsCmd.Find([]string{sub})
		if subCmd == nil {
			t.Fatalf("teams %s command not found", sub)
		}
	}

	createCmd, _, _ := teamsCmd.Find([]string{"create"})
	err := createCmd.ParseFlags([]string{"--data", "team.json"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	dataFile, _ := createCmd.Flags().GetString("data")
	if dataFile != "team.json" {
		t.Errorf("expected 'team.json', got %q", dataFile)
	}

	deleteCmd, _, _ := teamsCmd.Find([]string{"delete"})
	err = deleteCmd.ParseFlags([]string{"--force"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	force, _ := deleteCmd.Flags().GetBool("force")
	if !force {
		t.Error("expected --force to be true")
	}
}

func TestUserSubcommandsFlagsParsed(t *testing.T) {
	rootCmd := &cobra.Command{Use: "port"}
	RegisterAPI(rootCmd)

	apiCmd, _, _ := rootCmd.Find([]string{"api"})
	usersCmd, _, _ := apiCmd.Find([]string{"users"})
	if usersCmd == nil {
		t.Fatal("users command not found")
	}

	listCmd, _, _ := usersCmd.Find([]string{"list"})
	if listCmd == nil {
		t.Fatal("users list command not found")
	}

	getCmd, _, _ := usersCmd.Find([]string{"get"})
	if getCmd == nil {
		t.Fatal("users get command not found")
	}

	err := listCmd.ParseFlags([]string{"--format", "yaml"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	format, _ := listCmd.Flags().GetString("format")
	if format != "yaml" {
		t.Errorf("expected 'yaml', got %q", format)
	}
}

func TestScorecardSubcommandsFlagsParsed(t *testing.T) {
	rootCmd := &cobra.Command{Use: "port"}
	RegisterAPI(rootCmd)

	apiCmd, _, _ := rootCmd.Find([]string{"api"})
	scorecardsCmd, _, _ := apiCmd.Find([]string{"scorecards"})
	if scorecardsCmd == nil {
		t.Fatal("scorecards command not found")
	}

	for _, sub := range []string{"list", "create", "update", "delete"} {
		subCmd, _, _ := scorecardsCmd.Find([]string{sub})
		if subCmd == nil {
			t.Fatalf("scorecards %s command not found", sub)
		}
	}

	listCmd, _, _ := scorecardsCmd.Find([]string{"list"})
	err := listCmd.ParseFlags([]string{"--blueprint", "service"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	bp, _ := listCmd.Flags().GetString("blueprint")
	if bp != "service" {
		t.Errorf("expected 'service', got %q", bp)
	}
}

func TestActionSubcommandsFlagsParsed(t *testing.T) {
	rootCmd := &cobra.Command{Use: "port"}
	RegisterAPI(rootCmd)

	apiCmd, _, _ := rootCmd.Find([]string{"api"})
	actionsCmd, _, _ := apiCmd.Find([]string{"actions"})
	if actionsCmd == nil {
		t.Fatal("actions command not found")
	}

	for _, sub := range []string{"list", "create", "update", "delete"} {
		subCmd, _, _ := actionsCmd.Find([]string{sub})
		if subCmd == nil {
			t.Fatalf("actions %s command not found", sub)
		}
	}

	listCmd, _, _ := actionsCmd.Find([]string{"list"})
	err := listCmd.ParseFlags([]string{"--blueprint", "service"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	bp, _ := listCmd.Flags().GetString("blueprint")
	if bp != "service" {
		t.Errorf("expected 'service', got %q", bp)
	}
}

func TestAgentsSubcommandsFlagsParsed(t *testing.T) {
	rootCmd := &cobra.Command{Use: "port"}
	RegisterAPI(rootCmd)

	apiCmd, _, _ := rootCmd.Find([]string{"api"})
	agentsCmd, _, _ := apiCmd.Find([]string{"agents"})
	if agentsCmd == nil {
		t.Fatal("agents command not found")
	}

	invokeCmd, _, _ := agentsCmd.Find([]string{"invoke"})
	if invokeCmd == nil {
		t.Fatal("agents invoke command not found")
	}

	err := invokeCmd.ParseFlags([]string{"--data", "body.json", "--org", "myorg"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	dataFile, _ := invokeCmd.Flags().GetString("data")
	if dataFile != "body.json" {
		t.Errorf("expected 'body.json', got %q", dataFile)
	}

	org, _ := invokeCmd.Flags().GetString("org")
	if org != "myorg" {
		t.Errorf("expected 'myorg', got %q", org)
	}
}

func TestAISubcommandsFlagsParsed(t *testing.T) {
	rootCmd := &cobra.Command{Use: "port"}
	RegisterAPI(rootCmd)

	apiCmd, _, _ := rootCmd.Find([]string{"api"})
	aiCmd, _, _ := apiCmd.Find([]string{"ai"})
	if aiCmd == nil {
		t.Fatal("ai command not found")
	}

	invokeCmd, _, _ := aiCmd.Find([]string{"invoke"})
	if invokeCmd == nil {
		t.Fatal("ai invoke command not found")
	}

	err := invokeCmd.ParseFlags([]string{"--data", "prompt.json"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	dataFile, _ := invokeCmd.Flags().GetString("data")
	if dataFile != "prompt.json" {
		t.Errorf("expected 'prompt.json', got %q", dataFile)
	}

	getCmd, _, _ := aiCmd.Find([]string{"get"})
	if getCmd == nil {
		t.Fatal("ai get command not found")
	}

	err = getCmd.ParseFlags([]string{"--format", "yaml"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	format, _ := getCmd.Flags().GetString("format")
	if format != "yaml" {
		t.Errorf("expected 'yaml', got %q", format)
	}
}

func TestActionRunsSubcommandsFlagsParsed(t *testing.T) {
	rootCmd := &cobra.Command{Use: "port"}
	RegisterAPI(rootCmd)

	apiCmd, _, _ := rootCmd.Find([]string{"api"})
	actionRunsCmd, _, _ := apiCmd.Find([]string{"action-runs"})
	if actionRunsCmd == nil {
		t.Fatal("action-runs command not found")
	}

	for _, sub := range []string{"list", "get", "update", "approve", "execute"} {
		subCmd, _, _ := actionRunsCmd.Find([]string{sub})
		if subCmd == nil {
			t.Fatalf("action-runs %s command not found", sub)
		}
	}

	updateCmd, _, _ := actionRunsCmd.Find([]string{"update"})
	err := updateCmd.ParseFlags([]string{"--data", "run.json", "--org", "myorg"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	dataFile, _ := updateCmd.Flags().GetString("data")
	if dataFile != "run.json" {
		t.Errorf("expected 'run.json', got %q", dataFile)
	}

	listCmd, _, _ := actionRunsCmd.Find([]string{"list"})
	err = listCmd.ParseFlags([]string{"--format", "yaml"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	format, _ := listCmd.Flags().GetString("format")
	if format != "yaml" {
		t.Errorf("expected 'yaml', got %q", format)
	}
}

func TestPermissionsSubcommandsFlagsParsed(t *testing.T) {
	rootCmd := &cobra.Command{Use: "port"}
	RegisterAPI(rootCmd)

	apiCmd, _, _ := rootCmd.Find([]string{"api"})
	permsCmd, _, _ := apiCmd.Find([]string{"permissions"})
	if permsCmd == nil {
		t.Fatal("permissions command not found")
	}

	for _, resource := range []string{"blueprints", "actions", "pages"} {
		resourceCmd, _, _ := permsCmd.Find([]string{resource})
		if resourceCmd == nil {
			t.Fatalf("permissions %s command not found", resource)
		}
		for _, sub := range []string{"get", "update"} {
			subCmd, _, _ := resourceCmd.Find([]string{sub})
			if subCmd == nil {
				t.Fatalf("permissions %s %s command not found", resource, sub)
			}
		}
	}

	bpUpdateCmd, _, _ := permsCmd.Find([]string{"blueprints"})
	updateCmd, _, _ := bpUpdateCmd.Find([]string{"update"})
	err := updateCmd.ParseFlags([]string{"--data", "perms.json"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	dataFile, _ := updateCmd.Flags().GetString("data")
	if dataFile != "perms.json" {
		t.Errorf("expected 'perms.json', got %q", dataFile)
	}
}

func TestWebhooksSubcommandsFlagsParsed(t *testing.T) {
	rootCmd := &cobra.Command{Use: "port"}
	RegisterAPI(rootCmd)

	apiCmd, _, _ := rootCmd.Find([]string{"api"})
	webhooksCmd, _, _ := apiCmd.Find([]string{"webhooks"})
	if webhooksCmd == nil {
		t.Fatal("webhooks command not found")
	}

	for _, sub := range []string{"list", "get", "create", "update", "delete"} {
		subCmd, _, _ := webhooksCmd.Find([]string{sub})
		if subCmd == nil {
			t.Fatalf("webhooks %s command not found", sub)
		}
	}

	createCmd, _, _ := webhooksCmd.Find([]string{"create"})
	err := createCmd.ParseFlags([]string{"--data", "webhook.json"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	dataFile, _ := createCmd.Flags().GetString("data")
	if dataFile != "webhook.json" {
		t.Errorf("expected 'webhook.json', got %q", dataFile)
	}

	deleteCmd, _, _ := webhooksCmd.Find([]string{"delete"})
	err = deleteCmd.ParseFlags([]string{"--force"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	force, _ := deleteCmd.Flags().GetBool("force")
	if !force {
		t.Error("expected --force to be true")
	}
}

func TestAuditSubcommandsFlagsParsed(t *testing.T) {
	rootCmd := &cobra.Command{Use: "port"}
	RegisterAPI(rootCmd)

	apiCmd, _, _ := rootCmd.Find([]string{"api"})
	auditCmd, _, _ := apiCmd.Find([]string{"audit"})
	if auditCmd == nil {
		t.Fatal("audit command not found")
	}

	listCmd, _, _ := auditCmd.Find([]string{"list"})
	if listCmd == nil {
		t.Fatal("audit list command not found")
	}

	err := listCmd.ParseFlags([]string{"--format", "yaml", "--org", "myorg"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	format, _ := listCmd.Flags().GetString("format")
	if format != "yaml" {
		t.Errorf("expected 'yaml', got %q", format)
	}
}
