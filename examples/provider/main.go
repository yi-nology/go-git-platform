// Example: build a provider for any supported platform, list repositories,
// and use optional capability discovery for issues.
//
// Set PLATFORM_TOKEN to run. It makes one real API call to ListRepos.
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	_ "github.com/yi-nology/go-git-platform/backends/all" // register every platform
	"github.com/yi-nology/go-git-platform/model"
	"github.com/yi-nology/go-git-platform/provider"
)

func main() {
	token := os.Getenv("PLATFORM_TOKEN")
	if token == "" {
		log.Fatal("set PLATFORM_TOKEN to run this example")
	}

	// The same call works for any model.Platform — gitlab, gitea, gitee,
	// forgejo, gitcode, tencent_code.
	p, err := provider.NewProvider(provider.Config{
		Platform: model.PlatformGitHub,
		Token:    token,
	})
	if err != nil {
		log.Fatalf("new provider: %v", err)
	}

	ctx := context.Background()
	repos, err := p.ListRepos(ctx, provider.ListRepoOptions{Page: 1, PerPage: 5})
	if err != nil {
		log.Fatalf("list repos: %v", err)
	}
	for _, r := range repos {
		fmt.Println(r.FullName)
	}

	// IssueManager is an OPTIONAL capability (only some platforms implement it).
	// Type-assert instead of assuming every platform supports issues/search.
	if ism, ok := p.(provider.IssueManager); ok {
		issues, _, err := ism.ListIssues(ctx, provider.ListIssuesOptions{
			Owner: "golang", Repo: "go",
		})
		if err != nil {
			log.Printf("list issues: %v", err)
		} else {
			fmt.Printf("%d open issues\n", len(issues))
		}
	} else {
		fmt.Println("issues not supported on this platform")
	}
}
