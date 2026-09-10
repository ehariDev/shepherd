package simulate

// Exported for internal/simulate/worker_helpers_test.go (package
// simulate_test): classifySimulatorError has no business being part of this
// package's public API — nothing outside the worker calls it — but the spec
// that pins its error-mapping table lives in the external test package
// alongside the rest of this file's specs, so it needs a name it can reach.
var ClassifySimulatorError = classifySimulatorError
