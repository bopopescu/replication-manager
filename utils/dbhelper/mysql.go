// MySQL related functions

package dbhelper

import (
	"github.com/jmoiron/sqlx"
)

func HaveErrantTransactions(db *sqlx.DB, gtidMain string, gtidSubordinate string) (bool, string, error) {

	count := 0
	query := "select gtid_subset('" + gtidMain + "','" + gtidSubordinate + "') as subordinate_is_subset"

	err := db.QueryRowx(query).Scan(&count)
	if err != nil {
		return false, query, err
	}

	if count == 0 {
		return true, query, nil
	}
	return false, query, nil
}
