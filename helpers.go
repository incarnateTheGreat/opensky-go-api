package main

// =============================================================================
// HELPERS - Safe type conversions from interface{}
// =============================================================================
// These handle the fact that OpenSky returns mixed-type arrays
// In TypeScript you'd use type guards; in Go we use type assertions

func safeString(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func safeStringPtr(v interface{}) *string {
	if v == nil {
		return nil
	}
	if s, ok := v.(string); ok {
		if len(s) > 0 {
			return &s
		}
	}
	return nil
}

func safeFloat64Ptr(v interface{}) *float64 {
	if v == nil {
		return nil
	}
	// JSON numbers decode as float64 in Go
	if f, ok := v.(float64); ok {
		return &f
	}
	return nil
}

func safeInt64(v interface{}) int64 {
	if f, ok := v.(float64); ok {
		return int64(f)
	}
	return 0
}

func safeBool(v interface{}) bool {
	if b, ok := v.(bool); ok {
		return b
	}
	return false
}
