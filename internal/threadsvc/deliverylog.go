// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package threadsvc

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/faizalv/lemongrass/internal/atomicfile"
	"github.com/faizalv/lemongrass/internal/config"
)

// Past this size the log moves to a single .1 file, so it never grows without bound.
const deliveryLogLimit = 1 << 20

func deliveryLogPath() string {
	return filepath.Join(config.Dir(), "logs", "delivery.log")
}

// Best effort: the service and every posting command append here, and a failure never stops a delivery.
func appendDeliveryLog(tabID string, rowIDs []int64, outcome string) {
	path := deliveryLogPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return
	}
	if info, err := os.Stat(path); err == nil && info.Size() > deliveryLogLimit {
		atomicfile.Rename(path, path+".1")
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s tab=%s rows=%s %s\n", time.Now().UTC().Format(time.RFC3339Nano), tabID, joinIDs(rowIDs), strings.ReplaceAll(outcome, "\n", " "))
}

func joinIDs(ids []int64) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = fmt.Sprint(id)
	}
	return strings.Join(parts, ",")
}
