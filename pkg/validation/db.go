// Copyright (C) 2023 Haiko Schol
// SPDX-License-Identifier: GPL-3.0-or-later

// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.

// You should have received a copy of the GNU General Public License
// along with this program. If not, see <https://www.gnu.org/licenses/>.

package validation

import (
	"fmt"
	"os"
)

func ValidateDbEnv(dbHost, dbSSLMode, dbName, dbUser, dbPassword string) (errors []string) {
	if dbHost == "" {
		errors = append(errors, "COA_DB_HOST env var missing")
	}
	if dbSSLMode == "" {
		errors = append(errors, "COA_DB_SSLMODE env var missing")
	}
	if dbName == "" {
		errors = append(errors, "COA_DB_NAME env var missing")
	}
	if dbUser == "" {
		errors = append(errors, "COA_DB_USER env var missing")
	}
	if dbPassword == "" {
		errors = append(errors, "COA_DB_PASSWORD env var missing")
	}

	return errors
}

func LogErrors(errs []string, exit bool) {
	for _, e := range errs {
		fmt.Println(e)
	}

	if len(errs) > 0 && exit {
		os.Exit(1)
	}
}
