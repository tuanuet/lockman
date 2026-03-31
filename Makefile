.PHONY: test test-docs bench bench-baseline bench-redis lint tidy

test:
	go test ./...

test-docs:
	go test ./... -run 'TestProductionGuideForApplicationTeams|TestBenchmarkGuideForApplicationTeams|TestREADMELinksAdoptionDocs|TestBaselineBenchmarkContract|TestAdapterBenchmarkContract'

bench:
	@echo "Running adoption benchmarks..."
	go test -run '^$$' -bench '^BenchmarkAdoption' -benchmem .

bench-all:
	@echo "Running all adoption benchmarks (memory + redis)..."
	go test -run '^$$' -bench '^BenchmarkAdoption' -benchmem .

bench-baseline:
	@echo "Running baseline (memory) adoption benchmarks..."
	go test -run '^$$' -bench 'BenchmarkAdoptionRunMemory|BenchmarkAdoptionRunContentionMemory|BenchmarkAdoptionClaimMemory|BenchmarkAdoptionClaimDuplicateMemory|BenchmarkAdoptionStrictMemory|BenchmarkAdoptionCompositeMemory|BenchmarkAdoptionRenewalMemory' -benchmem .

bench-redis:
	@echo "Running Redis-backed adoption benchmarks..."
	go test -run '^$$' -bench 'BenchmarkAdoptionRunRedis|BenchmarkAdoptionClaimRedis|BenchmarkAdoptionStrictRedis|BenchmarkAdoptionCompositeRedis' -benchmem .

lint:
	go vet ./...
	gofmt -l .

tidy:
	go mod tidy
	go work sync
