package deployment

import (
	"fmt"

	"github.com/elijahmont3x/shipyard-action/pkg/config"
)

// resolveServiceDependencies sorts services based on their dependencies
func resolveServiceDependencies(services []config.Service) ([]config.Service, error) {
	// Extract dependency information into a simple graph structure
	names := make([]string, len(services))
	nameToIndex := make(map[string]int)
	dependencies := make(map[string][]string)

	for i, service := range services {
		names[i] = service.Name
		nameToIndex[service.Name] = i
		dependencies[service.Name] = service.DependsOn
	}

	// Use the core algorithm to get sorted order
	sortedNames, err := resolveDependencyOrder(names, dependencies)
	if err != nil {
		return nil, err
	}

	// Convert back to original type in sorted order
	result := make([]config.Service, len(sortedNames))
	for i, name := range sortedNames {
		result[i] = services[nameToIndex[name]]
	}

	return result, nil
}

// resolveAppDependencies sorts apps based on their dependencies
func resolveAppDependencies(apps []config.App) ([]config.App, error) {
	// Extract dependency information into a simple graph structure
	names := make([]string, len(apps))
	nameToIndex := make(map[string]int)
	dependencies := make(map[string][]string)

	for i, app := range apps {
		names[i] = app.Name
		nameToIndex[app.Name] = i
		dependencies[app.Name] = app.DependsOn
	}

	// Use the core algorithm to get sorted order
	sortedNames, err := resolveDependencyOrder(names, dependencies)
	if err != nil {
		return nil, err
	}

	// Convert back to original type in sorted order
	result := make([]config.App, len(sortedNames))
	for i, name := range sortedNames {
		result[i] = apps[nameToIndex[name]]
	}

	return result, nil
}

// resolveDependencyOrder is the core algorithm for topological sorting
// This works with any type that can be identified by string names
func resolveDependencyOrder(names []string, dependencies map[string][]string) ([]string, error) {
	// Perform topological sort
	visited := make(map[string]bool)
	temp := make(map[string]bool)
	order := []string{}

	// Visit function for DFS
	var visit func(string) error
	visit = func(name string) error {
		// Check for cycle
		if temp[name] {
			return fmt.Errorf("cycle detected in dependencies involving %s", name)
		}

		// Skip if already visited
		if visited[name] {
			return nil
		}

		// Mark as temporary visited
		temp[name] = true

		// Visit dependencies
		for _, dep := range dependencies[name] {
			if err := visit(dep); err != nil {
				return err
			}
		}

		// Mark as visited
		visited[name] = true
		temp[name] = false

		// Add to order
		order = append(order, name)

		return nil
	}

	// Visit all items
	for _, name := range names {
		if !visited[name] {
			if err := visit(name); err != nil {
				return nil, err
			}
		}
	}

	// Reverse the order (topological sort gives reverse dependency order)
	for i, j := 0, len(order)-1; i < j; i, j = i+1, j-1 {
		order[i], order[j] = order[j], order[i]
	}

	return order, nil
}
