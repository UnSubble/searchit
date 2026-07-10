package resolver

import (
	"fmt"

	"github.com/unsubble/searchit/internal/profile"
	"github.com/unsubble/searchit/internal/profile/types"
)

// Resolver resolves profile dependency graphs.
type Resolver struct {
	store profile.Store
}

// New creates a new dependency resolver.
func New(store profile.Store) *Resolver {
	return &Resolver{store: store}
}

// Resolve recursively resolves the dependencies of the given profiles, returning
// them in topological order (dependencies first, target profile last) with
// duplicates removed.
func (r *Resolver) Resolve(names []string) ([]*types.Profile, error) {
	var resolved []*types.Profile
	visiting := make(map[string]bool)
	visited := make(map[string]bool)
	var path []string

	var dfs func(name string) error
	dfs = func(name string) error {
		if visiting[name] {
			cycleStartIdx := -1
			for i, p := range path {
				if p == name {
					cycleStartIdx = i
					break
				}
			}
			var chain []string
			if cycleStartIdx != -1 {
				chain = append(chain, path[cycleStartIdx:]...)
			} else {
				chain = append(chain, path...)
			}
			chain = append(chain, name)
			return &CyclicDependencyError{
				Chain: chain,
			}
		}
		if visited[name] {
			return nil
		}

		p, err := r.store.Load(name)
		if err != nil {
			return fmt.Errorf("load dependency %q: %w", name, err)
		}

		visiting[name] = true
		path = append(path, name)

		for _, dep := range p.Depends {
			depResolvedName := resolveDepName(p.Name, dep)
			if err := dfs(depResolvedName); err != nil {
				return err
			}
		}

		visiting[name] = false
		path = path[:len(path)-1]

		visited[name] = true
		resolved = append(resolved, p)
		return nil
	}

	for _, name := range names {
		if err := dfs(name); err != nil {
			return nil, err
		}
	}

	return resolved, nil
}
