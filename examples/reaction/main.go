// Example: list reactions on an issue and add a reaction.
//
// Set PLATFORM_TOKEN to run. Works with GitHub, GitLab, Gitea, and Forgejo
// (issue #125: GitCode's reactions capability was removed — its wire routes
// live-404 and the type assertion fails).
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	_ "github.com/yi-nology/go-git-platform/backends/all" // register every platform
	"github.com/yi-nology/go-git-platform/provider"
)

func main() {
	token := os.Getenv("PLATFORM_TOKEN")
	if token == "" {
		log.Fatal("set PLATFORM_TOKEN to run this example")
	}

	p, err := provider.NewProvider(provider.Config{
		Platform: provider.PlatformGitHub,
		Token:    token,
	})
	if err != nil {
		log.Fatalf("new provider: %v", err)
	}

	// ReactionManager is an optional capability.
	rm, ok := p.(provider.ReactionManager)
	if !ok {
		fmt.Println("reactions not supported on this platform")
		return
	}

	ctx := context.Background()
	owner, repo, number := "golang", "go", "1"

	reactions, err := rm.ListIssueReactions(ctx, owner, repo, number)
	if err != nil {
		log.Fatalf("list reactions: %v", err)
	}

	counts := map[string]int{}
	for _, r := range reactions {
		counts[r.Emoji]++
	}
	fmt.Printf("Issue %s/%s#%s reactions:\n", owner, repo, number)
	for emoji, n := range counts {
		fmt.Printf("  %s: %d\n", emoji, n)
	}

	// Add a thumbs-up reaction.
	r, err := rm.AddIssueReaction(ctx, owner, repo, number, provider.ReactionPlusOne)
	if err != nil {
		log.Printf("add reaction: %v", err)
	} else {
		fmt.Printf("\nadded reaction: %s (id=%d)\n", r.Emoji, r.ID)
	}
}
