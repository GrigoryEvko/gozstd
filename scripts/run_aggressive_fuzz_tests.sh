#!/bin/bash

# Script to run all aggressive fuzz tests
# These tests are designed to find real bugs and edge cases

FUZZ_TIME="${1:-5m}"  # Default 5 minutes, can override with first argument

# All fuzz tests organized by category
declare -A FUZZ_CATEGORIES=(
    ["Memory Safety"]="FuzzDictionaryByRefMemorySafety FuzzCCtxPoolAbuse FuzzDoubleFree FuzzConcurrentPoolAccess FuzzUnsafePointerManipulation"
    ["Parameter Boundaries"]="FuzzParameterBoundaries FuzzConflictingParameters FuzzAllParameterCombinations FuzzParameterOverflow FuzzMultiThreadingParameters"
    ["State Machine"]="FuzzStateMachineReset FuzzPledgedSizeLies FuzzContextReuse FuzzPoolContamination FuzzRapidContextOperations"
    ["Corrupted Data"]="FuzzBitFlipping FuzzTruncation FuzzHeaderCorruption FuzzDictionaryMismatch FuzzMixedStreams FuzzSizeFieldTampering"
    ["Race Conditions"]="FuzzSharedCCtxRace FuzzDictionaryReleaseRace FuzzPoolCorruption FuzzParameterRacing FuzzByRefDictionaryRace FuzzConcurrentReset"
    ["Resource Exhaustion"]="FuzzGiantInputs FuzzTinyBuffers FuzzDictionarySizeLimits FuzzInfiniteCompression FuzzMemoryPressure FuzzManySmallCompressions"
    ["Original Tests"]="FuzzCompressDecompress FuzzCompressWithBuffer FuzzDictionary FuzzDictionaryByRef FuzzAdvancedAPI FuzzStreamCompression FuzzConcurrentCompression FuzzResetContext FuzzPledgedSize FuzzInvalidInput"
)

echo "================================================================"
echo "AGGRESSIVE FUZZ TESTING SUITE"
echo "================================================================"
echo "Runtime per test: $FUZZ_TIME"
echo "Categories: ${#FUZZ_CATEGORIES[@]}"
echo "Total tests: $(for cat in "${!FUZZ_CATEGORIES[@]}"; do echo ${FUZZ_CATEGORIES[$cat]}; done | wc -w)"
echo "================================================================"
echo
echo "WARNING: These tests are designed to find bugs and may:"
echo "- Use race detector (slower but finds race conditions)"
echo "- Consume significant memory"
echo "- Trigger panics or crashes (which indicate bugs)"
echo "================================================================"
echo

# Create results directory
RESULTS_DIR="fuzz_results_aggressive_$(date +%Y%m%d_%H%M%S)"
mkdir -p "$RESULTS_DIR"

# Summary file
SUMMARY_FILE="$RESULTS_DIR/summary.txt"
echo "Aggressive Fuzz Test Summary - $(date)" > "$SUMMARY_FILE"
echo "=================================" >> "$SUMMARY_FILE"

# Run tests by category
for category in "${!FUZZ_CATEGORIES[@]}"; do
    echo
    echo "========================================"
    echo "CATEGORY: $category"
    echo "========================================"
    echo
    
    echo -e "\n$category" >> "$SUMMARY_FILE"
    echo "-------------------" >> "$SUMMARY_FILE"
    
    for test in ${FUZZ_CATEGORIES[$category]}; do
        echo "Running $test..."
        echo -n "  $test: " >> "$SUMMARY_FILE"
        
        LOG_FILE="$RESULTS_DIR/${category// /_}_${test}.log"
        
        # Run with race detector for race condition tests
        if [[ "$category" == "Race Conditions" ]]; then
            echo "  (with race detector)"
            if go test -race -fuzz=^$test\$ -fuzztime=$FUZZ_TIME -run=^$ -timeout=10m > "$LOG_FILE" 2>&1; then
                echo "  ✓ PASSED"
                echo "PASSED" >> "$SUMMARY_FILE"
            else
                echo "  ✗ FAILED (check $LOG_FILE)"
                echo "FAILED" >> "$SUMMARY_FILE"
                
                # Extract interesting failures
                echo "    Errors found:" | tee -a "$SUMMARY_FILE"
                grep -E "panic:|FAIL:|race:|error:" "$LOG_FILE" | head -5 | sed 's/^/    /' | tee -a "$SUMMARY_FILE"
            fi
        else
            # Run without race detector for better performance
            if go test -fuzz=^$test\$ -fuzztime=$FUZZ_TIME -run=^$ -timeout=10m > "$LOG_FILE" 2>&1; then
                echo "  ✓ PASSED"
                echo "PASSED" >> "$SUMMARY_FILE"
            else
                echo "  ✗ FAILED (check $LOG_FILE)"
                echo "FAILED" >> "$SUMMARY_FILE"
                
                # Extract interesting failures
                echo "    Errors found:" | tee -a "$SUMMARY_FILE"
                grep -E "panic:|FAIL:|error:" "$LOG_FILE" | head -5 | sed 's/^/    /' | tee -a "$SUMMARY_FILE"
            fi
        fi
        
        # Save any crash files
        if ls testdata/fuzz/$test/* 2>/dev/null; then
            mkdir -p "$RESULTS_DIR/crashes/$test"
            cp -r testdata/fuzz/$test/* "$RESULTS_DIR/crashes/$test/"
            echo "    Found crash inputs in $RESULTS_DIR/crashes/$test"
        fi
    done
done

echo
echo "================================================================"
echo "FUZZ TESTING COMPLETE"
echo "================================================================"
echo "Results saved in: $RESULTS_DIR"
echo "Summary available at: $SUMMARY_FILE"
echo

# Print summary
echo "Summary:"
echo "--------"
cat "$SUMMARY_FILE" | grep -E "PASSED|FAILED" | sort | uniq -c

# Check for any crashes
if find "$RESULTS_DIR/crashes" -type f 2>/dev/null | grep -q .; then
    echo
    echo "WARNING: Crash inputs found!"
    echo "These indicate bugs that should be investigated:"
    find "$RESULTS_DIR/crashes" -type f | head -10
fi

# Look for specific bug indicators
echo
echo "Potential bugs found:"
echo "--------------------"
grep -h -E "panic:|race:|use of freed|corruption|leak" "$RESULTS_DIR"/*.log | sort | uniq -c | head -20

echo
echo "To reproduce a failure:"
echo "  go test -fuzz=FuzzTestName -run=FuzzTestName/seed"