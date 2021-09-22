package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Read-only maps containing all environment variables
var strEnvMap = make(map[string]string)
var boolEnvMap = make(map[string]bool)
var intEnvMap = make(map[string]int)
var uintEnvMap = make(map[string]uint)
var int64EnvMap = make(map[string]int64)
var timeEnvMap = make(map[string]time.Duration)

// Mutex protecting one-time map store operation
var envMapMutex sync.RWMutex

func init() {
	for _, entry := range os.Environ() {
		pair := strings.Split(entry, "=")
		if len(pair) < 3 {
			strEnvMap[pair[0]] = pair[1]
		} else {
			strEnvMap[pair[0]] = strings.Join(pair[1:], "=")
		}
	}
}

// GetString returns a setting in string.
func GetString(key string, defaultValue ...string) string {
	envMapMutex.RLock()
	defer envMapMutex.RUnlock()

	val, exists := strEnvMap[key]
	if !exists {
		if len(defaultValue) == 0 {
			panic(fmt.Errorf("setting %s does not exist", key))
		}
		val = defaultValue[0]
	}

	return val
}

// GetBool returns a setting in bool.
func GetBool(key string, def ...bool) bool {
	envMapMutex.RLock()
	if boolVal, exists := boolEnvMap[key]; exists {
		envMapMutex.RUnlock()
		return boolVal
	}

	strVal, strExists := strEnvMap[key]
	if !strExists {
		envMapMutex.RUnlock()
		if len(def) == 0 {
			panic(fmt.Errorf("setting %s does not exist", key))
		}
		return def[0]
	}
	envMapMutex.RUnlock()

	if result, err := strconv.ParseBool(strVal); err != nil {
		panic(fmt.Errorf("failed to parse bool for setting %s, err=%w", key, err))
	} else {
		envMapMutex.Lock()
		boolEnvMap[key] = result
		envMapMutex.Unlock()
	}

	return boolEnvMap[key]
}

// GetInt returns a setting in integer.
func GetInt(key string, def ...int) int {
	envMapMutex.RLock()
	if intVal, exists := intEnvMap[key]; exists {
		envMapMutex.RUnlock()
		return intVal
	}

	strVal, strExists := strEnvMap[key]
	if !strExists {
		envMapMutex.RUnlock()
		if len(def) == 0 {
			panic(fmt.Errorf("setting %s does not exist", key))
		}
		return def[0]
	}
	envMapMutex.RUnlock()

	if result, err := strconv.ParseInt(strVal, 0, 32); err != nil {
		panic(fmt.Errorf("failed to parse int for setting %s, err=%w", key, err))
	} else {
		envMapMutex.Lock()
		intEnvMap[key] = int(result)
		envMapMutex.Unlock()
	}

	return intEnvMap[key]
}

// GetUint returns a setting in unsigned integer.
func GetUint(key string, def ...uint) uint {
	envMapMutex.RLock()
	if uintVal, exists := uintEnvMap[key]; exists {
		envMapMutex.RUnlock()
		return uintVal
	}

	strVal, strExists := strEnvMap[key]
	if !strExists {
		envMapMutex.RUnlock()
		if len(def) == 0 {
			panic(fmt.Errorf("setting %s does not exist", key))
		}
		return def[0]
	}
	envMapMutex.RUnlock()

	if result, err := strconv.ParseUint(strVal, 0, 32); err != nil {
		panic(fmt.Errorf("failed to parse uint for setting %s, err=%w", key, err))
	} else {
		envMapMutex.Lock()
		uintEnvMap[key] = uint(result)
		envMapMutex.Unlock()
	}

	return uintEnvMap[key]
}

// GetInt64 returns a setting in integer.
func GetInt64(key string, def ...int64) int64 {
	envMapMutex.RLock()
	if int64Val, exists := int64EnvMap[key]; exists {
		envMapMutex.RUnlock()
		return int64Val
	}

	strVal, strExists := strEnvMap[key]
	if !strExists {
		envMapMutex.RUnlock()
		if len(def) == 0 {
			panic(fmt.Errorf("setting %s does not exist", key))
		}
		return def[0]
	}
	envMapMutex.RUnlock()

	if result, err := strconv.ParseInt(strVal, 0, 64); err != nil {
		panic(fmt.Errorf("failed to parse int64 for setting %s, err=%w", key, err))
	} else {
		envMapMutex.Lock()
		int64EnvMap[key] = int64(result)
		envMapMutex.Unlock()
	}

	return int64EnvMap[key]
}

// GetMillisecond returns a setting in time.Duration.
func GetMillisecond(key string, def ...time.Duration) time.Duration {
	envMapMutex.RLock()
	if timeVal, exists := timeEnvMap[key]; exists {
		envMapMutex.RUnlock()
		return timeVal
	}

	strVal, strExists := strEnvMap[key]
	if !strExists {
		envMapMutex.RUnlock()
		if len(def) == 0 {
			panic(fmt.Errorf("setting %s does not exist", key))
		}
		return def[0]
	}
	envMapMutex.RUnlock()

	if uintVal, err := strconv.ParseUint(strVal, 0, 32); err != nil {
		panic(fmt.Errorf("failed to parse time for setting %s, err=%w", key, err))
	} else {
		envMapMutex.Lock()
		timeEnvMap[key] = time.Duration(uintVal) * time.Millisecond
		envMapMutex.Unlock()
	}

	return timeEnvMap[key]
}

// SetString sets string to strEnvMap.
func SetString(key string, value string) {
	envMapMutex.Lock()
	strEnvMap[key] = value
	envMapMutex.Unlock()
}
