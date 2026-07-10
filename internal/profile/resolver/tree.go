package resolver

import (
	"strings"

	"github.com/unsubble/searchit/internal/profile/types"
)

// Node represents a node in the profile dependency tree.
type Node struct {
	Profile  *types.Profile
	Children []*Node
}

// BuildTree constructs a dependency tree starting from rootName.
// It verifies that there are no dependency cycles or missing profiles
// using the existing Resolve verification.
func (r *Resolver) BuildTree(rootName string) (*Node, error) {
	// 1. Verify that the dependency tree is cycle-free and all dependencies can be loaded.
	if _, err := r.Resolve([]string{rootName}); err != nil {
		return nil, err
	}

	// 2. Recursively build the tree. Since we already validated the tree is a DAG
	// and all nodes are loadable, this will not infinite loop or error on loading.
	var build func(name string) (*Node, error)
	build = func(name string) (*Node, error) {
		p, err := r.store.Load(name)
		if err != nil {
			return nil, err
		}

		node := &Node{
			Profile: p,
		}

		for _, dep := range p.Depends {
			depResolvedName := resolveDepName(p.Name, dep)
			childNode, err := build(depResolvedName)
			if err != nil {
				return nil, err
			}
			node.Children = append(node.Children, childNode)
		}

		return node, nil
	}

	return build(rootName)
}

// RenderTree returns the ASCII tree representation of the Node.
func RenderTree(node *Node) string {
	var sb strings.Builder
	var render func(n *Node, prefix string, isLast bool)
	render = func(n *Node, prefix string, isLast bool) {
		sb.WriteString(n.Profile.Name + "\n")

		numChildren := len(n.Children)
		for i, child := range n.Children {
			childIsLast := i == numChildren-1

			sb.WriteString(prefix)
			if childIsLast {
				sb.WriteString("└── ")
			} else {
				sb.WriteString("├── ")
			}

			childPrefix := prefix
			if childIsLast {
				childPrefix += "    "
			} else {
				childPrefix += "│   "
			}

			render(child, childPrefix, childIsLast)
		}
	}

	render(node, "", true)
	return strings.TrimSuffix(sb.String(), "\n")
}
