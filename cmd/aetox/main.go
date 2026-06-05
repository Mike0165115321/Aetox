package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"aetox-cli/internal/contracts"
	"aetox-cli/internal/critic"
	"aetox-cli/internal/dispatcher"
	"aetox-cli/internal/executor"
	"aetox-cli/internal/memory"
	"aetox-cli/internal/planner"
	"aetox-cli/internal/tools"
	"aetox-cli/internal/config"
	"aetox-cli/internal/safety"
)

func main() {
	var autoApprove bool
	var rootPath string
	var maxRetries int
	var approvalTimeout int

	flag.BoolVar(&autoApprove, "yes", false, "auto approve all risky actions")
	flag.StringVar(&rootPath, "root", "", "optional sandbox root directory (default: current directory)")
	flag.IntVar(&maxRetries, "retries", 2, "max retry count for a failed step")
	flag.IntVar(&approvalTimeout, "approval-timeout", 60, "approval timeout in seconds")
	flag.Parse()

	goal := ""
	if flag.NArg() > 0 {
		for i, arg := range flag.Args() {
			if i > 0 {
				goal += " "
			}
			goal += arg
		}
	}
	goal = strings.TrimSpace(goal)
	if goal == "" {
		fmt.Println("Usage: aetox [--yes] \"your goal\"")
		os.Exit(2)
	}

	cfg := config.Load(config.ConfigOptions{
		RootPath:        rootPath,
		AutoApprove:     autoApprove,
		MaxRetries:      maxRetries,
		ApprovalTimeout: approvalTimeout,
	})

	ctx := context.Background()

	plan, err := planner.New().BuildPlan(ctx, goal, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Planning failed: %v\n", err)
		os.Exit(1)
	}

	registry := tools.NewRegistry()
	registry.Register(tools.NewFileListTool(cfg.SandboxRoot))
	registry.Register(tools.NewShellRunTool(cfg.SandboxRoot))
	registry.Register(tools.NewWebFetchTool())

	runCtx := memory.NewSessionContext(plan.ID, goal)
	dispatch := dispatcher.NewDispatcher(
		executor.NewExecutor(registry),
		critic.NewCritic(),
		safety.NewManager(cfg),
		runCtx,
		cfg.MaxRetries,
	)

	report, err := dispatch.Run(ctx, plan)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Dispatch failed: %v\n", err)
		if report != nil {
			printReport(plan, report)
		}
		os.Exit(1)
	}

	printReport(plan, report)
	if !report.Success {
		os.Exit(1)
	}
}

func printReport(plan contracts.TaskPlan, report *dispatcher.Report) {
	fmt.Printf("Goal: %s\n", plan.Goal)
	fmt.Printf("Plan ID: %s\n", plan.ID)
	fmt.Printf("Steps: %d\n", len(plan.Steps))
	fmt.Printf("Risk: %s\n", plan.RiskLevel)
	fmt.Printf("Requires approval: %t\n", plan.RequiresPermission)
	fmt.Println("Execution:")
	for _, stepResult := range report.StepResults {
		status := "FAIL"
		if stepResult.Status == "success" {
			status = "OK"
		}
		fmt.Printf(" - %s (step %d)\n", status, stepResult.StepID)
		if stepResult.Error != "" {
			fmt.Printf("   error: %s\n", stepResult.Error)
		}
		if stepResult.Output != "" {
			fmt.Printf("   output:\n%s\n", stepResult.Output)
		}
	}
	fmt.Printf("Success: %t\n", report.Success)
}
