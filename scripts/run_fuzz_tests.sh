#!/bin/bash

# Script to run all fuzz tests for 5 minutes each
# Total runtime: ~50 minutes for 10 tests

FUZZ_TIME="5m"
FUZZ_TESTS=(
    "FuzzCompressDecompress"
    "FuzzCompressWithBuffer"
    "FuzzDictionary"
    "FuzzDictionaryByRef"
    "FuzzAdvancedAPI"
    "FuzzStreamCompression"
    "FuzzConcurrentCompression"
    "FuzzResetContext"
    "FuzzPledgedSize"
    "FuzzInvalidInput"
)

echo "Starting fuzz tests - each will run for $FUZZ_TIME"
echo "Total expected runtime: ~$(( ${#FUZZ_TESTS[@]} * 5 )) minutes"
echo "============================================="

# Create directory for fuzz artifacts
mkdir -p fuzz_results

# Run each fuzz test
for test in "${FUZZ_TESTS[@]}"; do
    echo
    echo "Running $test for $FUZZ_TIME..."
    echo "Start time: $(date)"
    
    # Run the fuzz test and capture output
    if go test -fuzz=$test -fuzztime=$FUZZ_TIME -run=^$ 2>&1 | tee fuzz_results/${test}.log; then
        echo "✓ $test completed successfully"
    else
        echo "✗ $test failed or found issues"
    fi
    
    echo "End time: $(date)"
    echo "============================================="
done

echo
echo "All fuzz tests completed!"
echo "Results saved in fuzz_results/"

# Summary
echo
echo "Summary:"
echo "--------"
for test in "${FUZZ_TESTS[@]}"; do
    if grep -q "FAIL" fuzz_results/${test}.log 2>/dev/null; then
        echo "✗ $test: FAILED"
    else
        echo "✓ $test: PASSED"
    fi
done