.PHONY: test test-docs bench bench-baseline bench-redis lint tidy

test:
	go test ./...

test-docs:
	go test ./... -run 'TestProductionGuideForApplicationTeams|TestBenchmarkGuideForApplicationTeams|TestREADMELinksAdoptionDocs'

bench:
	@echo "Running adoption benchmarks..."
	go test -run '^$$' -bench '^BenchmarkAdoption' -benchmem ./benchmarks

bench-baseline:
	@echo "Running baseline (memory) adoption benchmarks..."
	go test -run '^$$' -bench 'BenchmarkAdoptionRunMemory|BenchmarkAdoptionRunContentionMemory|BenchmarkAdoptionClaimMemory|BenchmarkAdoptionClaimDuplicateMemory|BenchmarkAdoptionStrictMemory|BenchmarkAdoptionCompositeMemory|BenchmarkAdoptionRenewalMemory' -benchmem ./benchmarks

bench-redis:
	@echo "Running Redis-backed adoption benchmarks..."
	go test -run '^$$' -bench 'BenchmarkAdoptionRunRedis|BenchmarkAdoptionClaimRedis|BenchmarkAdoptionStrictRedis|BenchmarkAdoptionCompositeRedis' -benchmem ./benchmarks

lint:
	go vet ./...
	gofmt -l .

tidy:
	go mod tidy
	go work sync
