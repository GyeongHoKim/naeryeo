package main

// staticProvider wraps a findRoute fake as a resolver that always succeeds at
// resolution, so a test can exercise search behavior without also modelling
// provider selection.
func staticProvider(find routeFinder) providerResolver {
	return func() (routeFinder, *failure) { return find, nil }
}

// failingProvider models a run that never reaches a search: no provider
// configured, no key stored, or an unreadable keychain.
func failingProvider(f failure) providerResolver {
	return func() (routeFinder, *failure) { return nil, &f }
}

func geoPresent() bool { return true }
func geoAbsent() bool  { return false }
