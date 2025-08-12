package gozstd

import (
	"fmt"
	"runtime"
)

type CParameter int

// The right way to make these enums is by importing the zstd.h header and assigning
//   their values from the cgo interface. However, I cannot for the life of me figure
//   out how to actually get cgo to do this.

const (
	/* compression parameters
	 * Note: When compressing with a ZSTD_CDict these parameters are superseded
	 * by the parameters used to construct the ZSTD_CDict.
	 * See ZSTD_CCtx_refCDict() for more info (superseded-by-cdict). */
	/* Set compression parameters according to pre-defined cLevel table.
	 * Note that exact compression parameters are dynamically determined,
	 * depending on both compression level and srcSize (when known).
	 * Default level is ZSTD_CLEVEL_DEFAULT==3.
	 * Special: value 0 means default, which is controlled by ZSTD_CLEVEL_DEFAULT.
	 * Note 1 : it's possible to pass a negative compression level.
	 * Note 2 : setting a level resets all other compression parameters to default */
	ZSTD_c_compressionLevel = 100

	/* Advanced compression parameters :
	 * It's possible to pin down compression parameters to some specific values.
	 * In which case, these values are no longer dynamically selected by the compressor */

	/* Maximum allowed back-reference distance, expressed as power of 2.
	 * This will set a memory budget for streaming decompression,
	 * with larger values requiring more memory
	 * and typically compressing more.
	 * Must be clamped between ZSTD_WINDOWLOG_MIN and ZSTD_WINDOWLOG_MAX.
	 * Special: value 0 means "use default windowLog".
	 * Note: Using a windowLog greater than ZSTD_WINDOWLOG_LIMIT_DEFAULT
	 *       requires explicitly allowing such size at streaming decompression stage. */
	ZSTD_c_windowLog = 101

	/* Size of the initial probe table, as a power of 2.
	 * Resulting memory usage is (1 << (hashLog+2)).
	 * Must be clamped between ZSTD_HASHLOG_MIN and ZSTD_HASHLOG_MAX.
	 * Larger tables improve compression ratio of strategies <= dFast,
	 * and improve speed of strategies > dFast.
	 * Special: value 0 means "use default hashLog". */
	ZSTD_c_hashLog = 102

	/* Size of the multi-probe search table, as a power of 2.
	 * Resulting memory usage is (1 << (chainLog+2)).
	 * Must be clamped between ZSTD_CHAINLOG_MIN and ZSTD_CHAINLOG_MAX.
	 * Larger tables result in better and slower compression.
	 * This parameter is useless for "fast" strategy.
	 * It's still useful when using "dfast" strategy,
	 * in which case it defines a secondary probe table.
	 * Special: value 0 means "use default chainLog". */
	ZSTD_c_chainLog = 103

	/* Number of search attempts, as a power of 2.
	 * More attempts result in better and slower compression.
	 * This parameter is useless for "fast" and "dFast" strategies.
	 * Special: value 0 means "use default searchLog". */
	ZSTD_c_searchLog = 104

	/* Minimum size of searched matches.
	 * Note that Zstandard can still find matches of smaller size,
	 * it just tweaks its search algorithm to look for this size and larger.
	 * Larger values increase compression and decompression speed, but decrease ratio.
	 * Must be clamped between ZSTD_MINMATCH_MIN and ZSTD_MINMATCH_MAX.
	 * Note that currently, for all strategies < btopt, effective minimum is 4.
	 *                    , for all strategies > fast, effective maximum is 6.
	 * Special: value 0 means "use default minMatchLength". */
	ZSTD_c_minMatch = 105

	/* Impact of this field depends on strategy.
	 * For strategies btopt, btultra & btultra2:
	 *     Length of Match considered "good enough" to stop search.
	 *     Larger values make compression stronger, and slower.
	 * For strategy fast:
	 *     Distance between match sampling.
	 *     Larger values make compression faster, and weaker.
	 * Special: value 0 means "use default targetLength". */
	ZSTD_c_targetLength = 106

	/* See ZSTD_strategy enum definition.
	 * The higher the value of selected strategy, the more complex it is,
	 * resulting in stronger and slower compression.
	 * Special: value 0 means "use default strategy". */
	ZSTD_c_strategy = 107

	/* LDM mode parameters */

	/* Enable long distance matching.
	 * This parameter is designed to improve compression ratio
	 * for large inputs, by finding large matches at long distance.
	 * It increases memory usage and window size.
	 * Note: enabling this parameter increases default ZSTD_c_windowLog to 128 MB
	 * except when expressly set to a different value.
	 * Note: will be enabled by default if ZSTD_c_windowLog >= 128 MB and
	 * compression strategy >= ZSTD_btopt (== compression level 16+) */
	ZSTD_c_enableLongDistanceMatching = 160

	/* Size of the table for long distance matching, as a power of 2.
	 * Larger values increase memory usage and compression ratio,
	 * but decrease compression speed.
	 * Must be clamped between ZSTD_HASHLOG_MIN and ZSTD_HASHLOG_MAX
	 * default: windowlog - 7.
	 * Special: value 0 means "automatically determine hashlog". */
	ZSTD_c_ldmHashLog = 161

	/* Minimum match size for long distance matcher.
	 * Larger/too small values usually decrease compression ratio.
	 * Must be clamped between ZSTD_LDM_MINMATCH_MIN and ZSTD_LDM_MINMATCH_MAX.
	 * Special: value 0 means "use default value" (default: 64). */
	ZSTD_c_ldmMinMatch = 162

	/* Log size of each bucket in the LDM hash table for collision resolution.
	 * Larger values improve collision resolution but decrease compression speed.
	 * The maximum value is ZSTD_LDM_BUCKETSIZELOG_MAX.
	 * Special: value 0 means "use default value" (default: 3). */
	ZSTD_c_ldmBucketSizeLog = 163

	/* Frequency of inserting/looking up entries into the LDM hash table.
	 * Must be clamped between 0 and (ZSTD_WINDOWLOG_MAX - ZSTD_HASHLOG_MIN).
	 * Default is MAX(0, (windowLog - ldmHashLog)), optimizing hash table usage.
	 * Larger values improve compression speed.
	 * Deviating far from default value will likely result in a compression ratio decrease.
	 * Special: value 0 means "automatically determine hashRateLog". */
	ZSTD_c_ldmHashRateLog = 164

	/* frame parameters */

	/* Content size will be written into frame header _whenever known_ (default:1)
	 * Content size must be known at the beginning of compression.
	 * This is automatically the case when using ZSTD_compress2(),
	 * For streaming scenarios, content size must be provided with ZSTD_CCtx_setPledgedSrcSize() */
	ZSTD_c_contentSizeFlag = 200

	/* A 32-bits checksum of content is written at end of frame (default:0) */
	ZSTD_c_checksumFlag = 201

	/* When applicable, dictionary's ID is written into frame header (default:1) */
	ZSTD_c_dictIDFlag = 202

	/* Target compressed block size : v1.5.6+
	 * Attempts to fit compressed block size to be around targetCBlockSize.
	 * No guarantee on compressed block size (default:0 == disabled) */
	ZSTD_c_targetCBlockSize = 130

	/* Rsync-friendly mode : v1.5.6+
	 * Creates periodical synchronization points to make compressed data
	 * more suitable for rsync delta transfers (default:0 == disabled)
	 * NOTE: rsyncable mode only works when multithreading is enabled */
	ZSTD_c_rsyncable = 500

	/* Advanced streaming parameters */

	/* User's best guess of source size : experimental
	 * Hint is not valid when srcSizeHint == 0.
	 * Helps compression ratio when guess is close to actual source size */
	ZSTD_c_srcSizeHint = 1004

	/* Stable input buffer : experimental
	 * Tells compressor that input data will ALWAYS be the same between calls.
	 * Avoids memcpy() from input buffer but requires strict buffer management */
	ZSTD_c_stableInBuffer = 1006

	/* Stable output buffer : experimental
	 * Tells compressor that output buffer will not be resized between calls.
	 * Allows direct compression to output buffer without intermediate copies */
	ZSTD_c_stableOutBuffer = 1007

	/* External sequence producer API parameters */

	/* Block delimiters mode for sequence compression : experimental
	 * Controls how block boundaries are handled in sequence arrays */
	ZSTD_c_blockDelimiters = 1008

	/* Sequence validation : experimental
	 * Enable validation of external sequences for safety */
	ZSTD_c_validateSequences = 1009

	/* External sequence producer fallback : experimental
	 * Enable fallback to internal producer if external producer fails */
	ZSTD_c_enableSeqProducerFallback = 1014

	/* multi-threading parameters */
	/* These parameters are only useful if multi-threading is enabled (compiled with build macro ZSTD_MULTITHREAD).
	 * They return an error otherwise. */

	/* Select how many threads will be spawned to compress in parallel.
	 * When nbWorkers >= 1, triggers asynchronous mode when used with ZSTD_compressStream*() :
	 * ZSTD_compressStream*() consumes input and flush output if possible, but immediately gives back control to caller,
	 * while compression work is performed in parallel, within worker threads.
	 * (note : a strong exception to this rule is when first invocation of ZSTD_compressStream2() sets ZSTD_e_end :
	 *  in which case, ZSTD_compressStream2() delegates to ZSTD_compress2(), which is always a blocking call).
	 * More workers improve speed, but also increase memory usage.
	 * Default value is `0`, aka "single-threaded mode" : no worker is spawned, compression is performed inside Caller's thread, all invocations are blocking */
	ZSTD_c_nbWorkers = 400

	/* Size of a compression job. This value is enforced only when nbWorkers >= 1.
	 * Each compression job is completed in parallel, so this value can indirectly impact the nb of active threads.
	 * 0 means default, which is dynamically determined based on compression parameters.
	 * Job size must be a minimum of overlap size, or 1 MB, whichever is largest.
	 * The minimum size is automatically and transparently enforced. */
	ZSTD_c_jobSize = 401

	/* Control the overlap size, as a fraction of window size.
	 * The overlap size is an amount of data reloaded from previous job at the beginning of a new job.
	 * It helps preserve compression ratio, while each job is compressed in parallel.
	 * This value is enforced only when nbWorkers >= 1.
	 * Larger values increase compression ratio, but decrease speed.
	 * Possible values range from 0 to 9 :
	 * - 0 means "default" : value will be determined by the library, depending on strategy
	 * - 1 means "no overlap"
	 * - 9 means "full overlap", using a full window size.
	 * Each intermediate rank increases/decreases load size by a factor 2 :
	 * 9: full window;  8: w/2;  7: w/4;  6: w/8;  5:w/16;  4: w/32;  3:w/64;  2:w/128;  1:no overlap;  0:default
	 * default value varies between 6 and 9, depending on strategy */
	ZSTD_c_overlapLog = 402
)

type ZSTD_ResetDirective int

const (
	ZSTD_reset_session_only           = 1
	ZSTD_reset_parameters             = 2
	ZSTD_reset_session_and_parameters = 3
)

type ZSTD_SequenceFormat int

const (
	// ZSTD_Sequence[] has no block delimiters, just sequences
	ZSTD_sf_noBlockDelimiters = 0
	// ZSTD_Sequence[] contains explicit block delimiters
	ZSTD_sf_explicitBlockDelimiters = 1
)

type ZSTD_CompressionStrategy int

const (
	ZSTD_fast     = 1
	ZSTD_dfast    = 2
	ZSTD_greedy   = 3
	ZSTD_lazy     = 4
	ZSTD_lazy2    = 5
	ZSTD_btlazy2  = 6
	ZSTD_btopt    = 7
	ZSTD_btultra  = 8
	ZSTD_btultra2 = 9
	/* note : new strategies _might_ be added in the future.
	   Only the order (from fast to strong) is guaranteed */
)

// ===== Parameter Validation (merged from params_validation.go) =====

// ParameterBounds defines validation constraints for a ZSTD parameter
type ParameterBounds struct {
	Min         int
	Max         int
	Validator   func(value int, ctx *ValidationContext) error
	Description string
}

// ValidationContext provides context for parameter validation
type ValidationContext struct {
	Architecture    string
	CurrentParams   map[CParameter]int
	AvailableMemory uint64
	IsStreaming     bool
}

// PerformanceWarning represents a non-fatal performance concern
type PerformanceWarning struct {
	Type       string
	Message    string
	Suggestion string
}

// ParameterValidator handles comprehensive ZSTD parameter validation
type ParameterValidator struct {
	bounds map[CParameter]ParameterBounds
}

// NewParameterValidator creates a validator with architecture-aware bounds
func NewParameterValidator() *ParameterValidator {
	pv := &ParameterValidator{
		bounds: make(map[CParameter]ParameterBounds),
	}

	// Architecture-specific limits
	windowLogMax := 31
	chainLogMax := 30
	if runtime.GOARCH == "386" || runtime.GOARCH == "arm" {
		windowLogMax = 30
		chainLogMax = 29
	}

	// Core compression parameters with bounds
	pv.bounds[ZSTD_c_compressionLevel] = ParameterBounds{
		Min: -131072, Max: 22,
		Description: "Compression level: negative values = fast mode, positive = better compression",
	}

	pv.bounds[ZSTD_c_windowLog] = ParameterBounds{
		Min: 10, Max: windowLogMax,
		Validator:   validateMemoryImpact,
		Description: fmt.Sprintf("Window log: 10-%d (requires 2^windowLog bytes memory)", windowLogMax),
	}

	pv.bounds[ZSTD_c_hashLog] = ParameterBounds{
		Min: 6, Max: 30,
		Validator:   validateMemoryImpact,
		Description: "Hash log: 6-30 (requires 2^(hashLog+2) bytes memory)",
	}

	pv.bounds[ZSTD_c_chainLog] = ParameterBounds{
		Min: 6, Max: chainLogMax,
		Validator:   validateMemoryImpact,
		Description: fmt.Sprintf("Chain log: 6-%d (requires 2^(chainLog+2) bytes memory)", chainLogMax),
	}

	pv.bounds[ZSTD_c_searchLog] = ParameterBounds{
		Min: 1, Max: 30,
		Description: "Search log: 1-30 (number of search attempts)",
	}

	pv.bounds[ZSTD_c_minMatch] = ParameterBounds{
		Min: 3, Max: 7,
		Validator:   validateMinMatchForStrategy,
		Description: "Minimum match: 3-7 (strategy-dependent effective range)",
	}

	pv.bounds[ZSTD_c_targetLength] = ParameterBounds{
		Min: 0, Max: 131072,
		Description: "Target length: 0-131072 (strategy-dependent optimization target)",
	}

	pv.bounds[ZSTD_c_strategy] = ParameterBounds{
		Min: 1, Max: 9,
		Description: "Compression strategy: 1-9 (ZSTD_fast to ZSTD_btultra2)",
	}

	// LDM parameters
	pv.bounds[ZSTD_c_enableLongDistanceMatching] = ParameterBounds{
		Min: 0, Max: 1,
		Description: "Enable long distance matching: 0=disabled, 1=enabled",
	}

	pv.bounds[ZSTD_c_ldmHashLog] = ParameterBounds{
		Min: 6, Max: 30,
		Validator:   validateLDMConstraints,
		Description: "LDM hash log: 6-30 (must be <= windowLog when LDM enabled)",
	}

	pv.bounds[ZSTD_c_ldmMinMatch] = ParameterBounds{
		Min: 4, Max: 4096,
		Description: "LDM minimum match: 4-4096",
	}

	pv.bounds[ZSTD_c_ldmBucketSizeLog] = ParameterBounds{
		Min: 1, Max: 8,
		Description: "LDM bucket size log: 1-8",
	}

	pv.bounds[ZSTD_c_ldmHashRateLog] = ParameterBounds{
		Min: 0, Max: 30,
		Description: "LDM hash rate log: 0-30",
	}

	// Frame parameters
	pv.bounds[ZSTD_c_contentSizeFlag] = ParameterBounds{
		Min: 0, Max: 1,
		Description: "Content size flag: 0=disabled, 1=enabled",
	}

	pv.bounds[ZSTD_c_checksumFlag] = ParameterBounds{
		Min: 0, Max: 1,
		Description: "Checksum flag: 0=disabled, 1=enabled",
	}

	pv.bounds[ZSTD_c_dictIDFlag] = ParameterBounds{
		Min: 0, Max: 1,
		Description: "Dictionary ID flag: 0=disabled, 1=enabled",
	}

	// Block size control parameters
	pv.bounds[ZSTD_c_targetCBlockSize] = ParameterBounds{
		Min: 0, Max: 128 * 1024, // ZSTD_BLOCKSIZE_MAX is 128KB
		Description: "Target compressed block size: 0=auto, >0=target size in bytes",
	}

	pv.bounds[ZSTD_c_rsyncable] = ParameterBounds{
		Min: 0, Max: 1,
		Validator:   validateRsyncableRequirements,
		Description: "Rsync-friendly mode: 0=disabled, 1=enabled (requires multi-threading)",
	}

	// Advanced streaming parameters
	pv.bounds[ZSTD_c_srcSizeHint] = ParameterBounds{
		Min: 0, Max: 1 << 30, // 1GB reasonable maximum
		Description: "Source size hint: 0=no hint, >0=estimated input size in bytes",
	}

	pv.bounds[ZSTD_c_stableInBuffer] = ParameterBounds{
		Min: 0, Max: 1,
		Description: "Stable input buffer: 0=normal buffering, 1=user guarantees stable input",
	}

	pv.bounds[ZSTD_c_stableOutBuffer] = ParameterBounds{
		Min: 0, Max: 1,
		Description: "Stable output buffer: 0=normal buffering, 1=user guarantees stable output",
	}

	// External sequence producer API parameters
	pv.bounds[ZSTD_c_blockDelimiters] = ParameterBounds{
		Min: 0, Max: 1, // ZSTD_sf_noBlockDelimiters=0, ZSTD_sf_explicitBlockDelimiters=1
		Description: "Block delimiters mode: 0=no delimiters, 1=explicit delimiters",
	}

	pv.bounds[ZSTD_c_validateSequences] = ParameterBounds{
		Min: 0, Max: 1,
		Description: "Sequence validation: 0=disabled, 1=enabled (validates external sequences)",
	}

	pv.bounds[ZSTD_c_enableSeqProducerFallback] = ParameterBounds{
		Min: 0, Max: 1,
		Description: "Sequence producer fallback: 0=disabled, 1=enabled (fallback to internal producer)",
	}

	// Multi-threading parameters
	pv.bounds[ZSTD_c_nbWorkers] = ParameterBounds{
		Min: 0, Max: 200, // Reasonable upper bound
		Validator:   validateWorkerCount,
		Description: "Number of workers: 0=single-threaded, >0=multi-threaded",
	}

	pv.bounds[ZSTD_c_jobSize] = ParameterBounds{
		Min: 0, Max: 1 << 29, // 512MB reasonable max
		Description: "Job size: 0=auto, >0=manual size per job",
	}

	pv.bounds[ZSTD_c_overlapLog] = ParameterBounds{
		Min: 0, Max: 9,
		Description: "Overlap log: 0-9 (overlap fraction between jobs)",
	}

	return pv
}

// ValidateParameter performs comprehensive validation of a single parameter
func (pv *ParameterValidator) ValidateParameter(param CParameter, value int, ctx *ValidationContext) error {
	bounds, exists := pv.bounds[param]
	if !exists {
		// Allow unknown parameters to pass through to ZSTD for experimental features
		return nil
	}

	// Basic bounds checking
	if value < bounds.Min || value > bounds.Max {
		return &ParameterError{&ZstdError{
			Code:      42, // ZSTD_error_parameter_outOfBound
			Operation: "parameter validation",
			Message: fmt.Sprintf("parameter %v value %d outside valid range [%d, %d]",
				param, value, bounds.Min, bounds.Max),
			Recoverable: true,
			Suggestion: fmt.Sprintf("Use value between %d and %d. %s",
				bounds.Min, bounds.Max, bounds.Description),
			Context: ErrorContext{
				CompressionLevel: value,
			},
		}}
	}

	// Advanced validation if provided
	if bounds.Validator != nil {
		return bounds.Validator(value, ctx)
	}

	return nil
}

// ValidateParameterDependencies checks parameter interdependencies
func ValidateParameterDependencies(params map[CParameter]int) error {
	// LDM constraints
	if enableLDM, ldmEnabled := params[ZSTD_c_enableLongDistanceMatching]; ldmEnabled && enableLDM == 1 {
		if ldmHashLog, ok := params[ZSTD_c_ldmHashLog]; ok {
			if windowLog, ok := params[ZSTD_c_windowLog]; ok {
				if ldmHashLog > windowLog {
					return &ParameterError{&ZstdError{
						Code:      41, // ZSTD_error_parameter_combination_unsupported
						Operation: "parameter dependency validation",
						Message: fmt.Sprintf("ldmHashLog (%d) cannot exceed windowLog (%d)",
							ldmHashLog, windowLog),
						Recoverable: true,
						Suggestion: fmt.Sprintf("Set ldmHashLog <= %d or increase windowLog to >= %d",
							windowLog, ldmHashLog),
						Context: ErrorContext{
							CompressionLevel: ldmHashLog,
						},
					}}
				}
			}
		}
	}

	// Strategy-specific minMatch validation
	if strategy, strategySet := params[ZSTD_c_strategy]; strategySet {
		if minMatch, minMatchSet := params[ZSTD_c_minMatch]; minMatchSet {
			if err := validateMinMatchStrategy(strategy, minMatch); err != nil {
				return err
			}
		}
	}

	// Rsync-friendly mode validation - SAFETY CHECK
	if enableRsync, rsyncSet := params[ZSTD_c_rsyncable]; rsyncSet && enableRsync == 1 {
		// CRITICAL BUG WORKAROUND: Disable rsync mode due to ZSTD 1.5.7 segfault
		return &ParameterError{&ZstdError{
			Code:        41, // ZSTD_error_parameter_combination_unsupported
			Operation:   "rsyncable safety validation",
			Message:     "rsyncable mode temporarily disabled due to segfault bug in ZSTD 1.5.7 when combined with multi-threading. This is a safety measure to prevent application crashes.",
			Recoverable: true,
			Suggestion:  "Use standard compression without rsync mode, or wait for ZSTD library update.",
			Context: ErrorContext{
				CompressionLevel: enableRsync,
			},
		}}

		/* Original validation logic - re-enable when ZSTD bug is fixed:
		if nbWorkers, workersSet := params[ZSTD_c_nbWorkers]; !workersSet || nbWorkers == 0 {
			return &ParameterError{&ZstdError{
				Code: 41, // ZSTD_error_parameter_combination_unsupported
				Operation: "rsyncable dependency validation",
				Message: "rsyncable mode requires multi-threading (nbWorkers > 0)",
				Recoverable: true,
				Suggestion: "Set nbWorkers > 0 when enabling rsyncable mode",
				Context: ErrorContext{
					CompressionLevel: enableRsync,
				},
			}}
		}
		*/
	}

	// Memory budget validation - CRITICAL for security
	return validateMemoryRequirements(params)
}

// validateMemoryImpact validates individual parameter memory impact
func validateMemoryImpact(value int, ctx *ValidationContext) error {
	// This is called for windowLog, hashLog, chainLog
	// Calculate worst-case memory for this parameter
	var memoryEstimate uint64

	switch {
	case value <= 20:
		memoryEstimate = 1 << (value + 2) // Conservative estimate
	case value <= 25:
		memoryEstimate = 1 << (value + 3) // Higher memory overhead
	default:
		memoryEstimate = 1 << (value + 4) // Very high memory overhead
	}

	// Check against reasonable limits (1GB per parameter)
	if memoryEstimate > 1<<30 {
		return &MemoryError{&ZstdError{
			Code:      64, // ZSTD_error_memory_allocation
			Operation: "memory impact validation",
			Message: fmt.Sprintf("parameter value %d would require approximately %d MB memory",
				value, memoryEstimate/(1<<20)),
			Recoverable: true,
			Suggestion: fmt.Sprintf("Reduce parameter value to %d or less for reasonable memory usage",
				value-2),
			Context: ErrorContext{
				CompressionLevel: value,
			},
		}}
	}

	return nil
}

// validateMemoryRequirements validates total memory budget for parameter combination
func validateMemoryRequirements(params map[CParameter]int) error {
	var totalMemory uint64

	// Window size memory
	if windowLog, ok := params[ZSTD_c_windowLog]; ok && windowLog > 0 {
		totalMemory += 1 << windowLog
	}

	// Hash table memory
	if hashLog, ok := params[ZSTD_c_hashLog]; ok && hashLog > 0 {
		totalMemory += 1 << (hashLog + 2)
	}

	// Chain table memory
	if chainLog, ok := params[ZSTD_c_chainLog]; ok && chainLog > 0 {
		totalMemory += 1 << (chainLog + 2)
	}

	// LDM hash table memory
	if enableLDM, ldmEnabled := params[ZSTD_c_enableLongDistanceMatching]; ldmEnabled && enableLDM == 1 {
		if ldmHashLog, ok := params[ZSTD_c_ldmHashLog]; ok && ldmHashLog > 0 {
			totalMemory += 1 << (ldmHashLog + 2)
		}
	}

	// Get available system memory (simplified - in production should use actual system info)
	var systemMemory uint64 = 8 << 30 // Assume 8GB system - should be detected

	// Use at most 25% of system memory for ZSTD
	maxAllowed := systemMemory / 4

	if totalMemory > maxAllowed {
		return &MemoryError{&ZstdError{
			Code:      64, // ZSTD_error_memory_allocation
			Operation: "memory budget validation",
			Message: fmt.Sprintf("parameter combination requires %d MB (%.1f%% of assumed system memory)",
				totalMemory/(1<<20), float64(totalMemory)*100/float64(systemMemory)),
			Recoverable: true,
			Suggestion: fmt.Sprintf("Reduce windowLog, hashLog, or chainLog to use less than %d MB",
				maxAllowed/(1<<20)),
			Context: ErrorContext{},
		}}
	}

	return nil
}

// validateLDMConstraints validates LDM-specific constraints
func validateLDMConstraints(ldmHashLog int, ctx *ValidationContext) error {
	if ctx.CurrentParams == nil {
		return nil // Cannot validate without context
	}

	if enableLDM, ok := ctx.CurrentParams[ZSTD_c_enableLongDistanceMatching]; ok && enableLDM == 1 {
		if windowLog, ok := ctx.CurrentParams[ZSTD_c_windowLog]; ok {
			if ldmHashLog > windowLog {
				return &ParameterError{&ZstdError{
					Code:      41, // ZSTD_error_parameter_combination_unsupported
					Operation: "LDM constraint validation",
					Message: fmt.Sprintf("ldmHashLog (%d) cannot exceed windowLog (%d) when LDM is enabled",
						ldmHashLog, windowLog),
					Recoverable: true,
					Suggestion:  fmt.Sprintf("Set ldmHashLog <= %d or disable LDM", windowLog),
					Context: ErrorContext{
						CompressionLevel: ldmHashLog,
					},
				}}
			}
		}
	}

	return nil
}

// validateMinMatchForStrategy validates minMatch constraints for compression strategy
func validateMinMatchForStrategy(value int, ctx *ValidationContext) error {
	if ctx.CurrentParams == nil {
		return nil
	}

	if strategy, ok := ctx.CurrentParams[ZSTD_c_strategy]; ok {
		return validateMinMatchStrategy(strategy, value)
	}

	return nil
}

// validateMinMatchStrategy validates minMatch for a specific strategy
func validateMinMatchStrategy(strategy, minMatch int) error {
	// Strategy-specific minMatch constraints from ZSTD source
	switch strategy {
	case 1: // ZSTD_fast
		if minMatch > 7 {
			return &ParameterError{&ZstdError{
				Code:        42, // ZSTD_error_parameter_outOfBound
				Operation:   "strategy-specific validation",
				Message:     fmt.Sprintf("minMatch %d too large for ZSTD_fast strategy (max 7)", minMatch),
				Recoverable: true,
				Suggestion:  "Use minMatch <= 7 for ZSTD_fast strategy",
				Context: ErrorContext{
					CompressionLevel: minMatch,
				},
			}}
		}
	case 2, 3, 4, 5, 6, 7, 8, 9: // btopt+ strategies
		if minMatch < 3 {
			return &ParameterError{&ZstdError{
				Code:        42, // ZSTD_error_parameter_outOfBound
				Operation:   "strategy-specific validation",
				Message:     fmt.Sprintf("minMatch %d too small for btopt+ strategies (min 3)", minMatch),
				Recoverable: true,
				Suggestion:  "Use minMatch >= 3 for btopt+ strategies",
				Context: ErrorContext{
					CompressionLevel: minMatch,
				},
			}}
		}
		if minMatch > 6 {
			// This is more of a performance warning than hard error
			return &ParameterError{&ZstdError{
				Code:        42, // ZSTD_error_parameter_outOfBound
				Operation:   "strategy-specific validation",
				Message:     fmt.Sprintf("minMatch %d may reduce performance for btopt+ strategies", minMatch),
				Recoverable: true,
				Suggestion:  "Consider using minMatch 4-6 for optimal performance",
				Context: ErrorContext{
					CompressionLevel: minMatch,
				},
			}}
		}
	}

	return nil
}

// validateWorkerCount validates multi-threading worker count
func validateWorkerCount(workers int, ctx *ValidationContext) error {
	if workers > runtime.NumCPU()*2 {
		return &ParameterError{&ZstdError{
			Code:      42, // ZSTD_error_parameter_outOfBound
			Operation: "worker count validation",
			Message: fmt.Sprintf("nbWorkers (%d) significantly exceeds CPU count (%d)",
				workers, runtime.NumCPU()),
			Recoverable: true,
			Suggestion: fmt.Sprintf("Consider using at most %d workers for optimal performance",
				runtime.NumCPU()),
			Context: ErrorContext{
				CompressionLevel: workers,
			},
		}}
	}

	return nil
}

// CheckPerformanceWarnings identifies potential performance issues
func CheckPerformanceWarnings(params map[CParameter]int) []PerformanceWarning {
	var warnings []PerformanceWarning

	// Check for extremely high compression levels
	if level, ok := params[ZSTD_c_compressionLevel]; ok && level > 15 {
		warnings = append(warnings, PerformanceWarning{
			Type:       "high_compression_level",
			Message:    fmt.Sprintf("Compression level %d may be very slow", level),
			Suggestion: "Consider using level 6-12 for most use cases",
		})
	}

	// Check for excessive multi-threading
	if nbWorkers, ok := params[ZSTD_c_nbWorkers]; ok && nbWorkers > runtime.NumCPU() {
		warnings = append(warnings, PerformanceWarning{
			Type:       "excessive_workers",
			Message:    fmt.Sprintf("nbWorkers (%d) exceeds CPU count (%d)", nbWorkers, runtime.NumCPU()),
			Suggestion: fmt.Sprintf("Consider using at most %d workers", runtime.NumCPU()),
		})
	}

	// Check for memory-intensive parameter combinations
	var totalMemMB uint64
	if windowLog, ok := params[ZSTD_c_windowLog]; ok {
		totalMemMB += (1 << windowLog) / (1 << 20)
	}
	if hashLog, ok := params[ZSTD_c_hashLog]; ok {
		totalMemMB += (1 << (hashLog + 2)) / (1 << 20)
	}
	if chainLog, ok := params[ZSTD_c_chainLog]; ok {
		totalMemMB += (1 << (chainLog + 2)) / (1 << 20)
	}

	if totalMemMB > 1024 { // > 1GB
		warnings = append(warnings, PerformanceWarning{
			Type:       "high_memory_usage",
			Message:    fmt.Sprintf("Parameter combination may use %d MB memory", totalMemMB),
			Suggestion: "Consider reducing windowLog, hashLog, or chainLog for lower memory usage",
		})
	}

	return warnings
}

// validateRsyncableRequirements validates that rsyncable mode is only enabled with multi-threading
func validateRsyncableRequirements(value int, ctx *ValidationContext) error {
	if value == 0 {
		return nil // Disabled is always valid
	}

	// CRITICAL BUG WORKAROUND: ZSTD 1.5.7 has a known segfault issue when rsync-friendly mode
	// is combined with multi-threading in high-frequency compression scenarios (like benchmarks).
	// This causes NULL pointer dereference inside the ZSTD library itself.
	// Temporarily disable rsync mode until ZSTD is updated to a safe version.
	// See: https://github.com/facebook/zstd/issues - segfault in rsync + multi-threading
	return &ParameterError{&ZstdError{
		Code:        41, // ZSTD_error_parameter_combination_unsupported
		Operation:   "rsyncable safety validation",
		Message:     "rsyncable mode temporarily disabled due to segfault bug in ZSTD 1.5.7 when combined with multi-threading. This is a safety measure to prevent application crashes.",
		Recoverable: true,
		Suggestion:  "Use standard compression without rsync mode, or wait for ZSTD library update.",
		Context: ErrorContext{
			CompressionLevel: value,
		},
	}}

	/* Original validation logic - re-enable when ZSTD bug is fixed:

	if ctx.CurrentParams == nil {
		// Cannot validate without context - allow but will be checked at dependency validation
		return nil
	}

	// Check if multi-threading is enabled (nbWorkers > 0)
	nbWorkers, workersSet := ctx.CurrentParams[ZSTD_c_nbWorkers]
	if !workersSet || nbWorkers == 0 {
		return &ParameterError{&ZstdError{
			Code: 41, // ZSTD_error_parameter_combination_unsupported
			Operation: "rsyncable requirements validation",
			Message: "rsyncable mode requires multi-threading to be enabled (nbWorkers > 0)",
			Recoverable: true,
			Suggestion: "Set nbWorkers > 0 before enabling rsyncable mode, or disable rsyncable",
			Context: ErrorContext{
				CompressionLevel: value,
			},
		}}
	}

	return nil
	*/
}
