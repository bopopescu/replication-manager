package dbhelper

import "github.com/jmoiron/sqlx"
import log "github.com/sirupsen/logrus"

func SetHeartbeatTable(db *sqlx.DB) error {

	if db.DriverName() == "mysql" {
		stmt := "SET sql_log_bin=0"
		_, err := db.Exec(stmt)
		if err != nil {
			return err
		}

		stmt = "CREATE DATABASE IF NOT EXISTS replication_manager_schema"
		_, err = db.Exec(stmt)
		if err != nil {
			return err
		}
		stmt = "CREATE TABLE IF NOT EXISTS replication_manager_schema.heartbeat(secret varchar(64) ,cluster varchar(128),uid int , uuid varchar(128),  main varchar(128) , date timestamp,arbitration_date timestamp, status CHAR(1) DEFAULT 'U', hosts INT DEFAULT 0, failed INT DEFAULT 0, PRIMARY KEY(secret,cluster,uid) ) engine=innodb"
		_, err = db.Exec(stmt)
		if err != nil {
			return err
		}
		return nil
	}
	if db.DriverName() == "sqlite3" {
		stmt := `CREATE TABLE IF NOT EXISTS heartbeat(
			secret varchar(64),
			cluster varchar(128),
			uid int,
			uuid varchar(128),
			main varchar(128),
			date timestamp,
			arbitration_date timestamp,
			status CHAR(1) DEFAULT 'U',
			hosts INT DEFAULT 0,
			failed INT DEFAULT 0,
			PRIMARY KEY(secret,cluster,uid)
		)`
		_, err := db.Exec(stmt)
		if err != nil {
			return err
		}
	}
	return nil
}

func WriteHeartbeat(db *sqlx.DB, uuid string, secret string, cluster string, main string, uid int, hosts int, failed int) error {

	// stmt := "INSERT INTO heartbeat(secret,uuid,uid,main,date,cluster,hosts,failed) VALUES('" + secret + "','" + uuid + "'," + uid + ",'" + main + "', DATETIME('now'),'" + cluster + "'," + hosts + "," + failed
	stmt := "INSERT OR REPLACE INTO heartbeat (secret,uuid,uid,main,date,cluster,hosts,failed) VALUES(?,?,?,?,DATETIME('now'),?,?,?)"
	_, err := db.Exec(stmt, secret, uuid, uid, main, cluster, hosts, failed)
	if err != nil {
		return err
	}

	var count int
	stmt = "SELECT count(distinct main) FROM heartbeat WHERE cluster=? AND secret=? AND date > DATETIME('now', '-10 seconds')"
	err = db.QueryRowx(stmt, cluster, secret).Scan(&count)
	if err == nil && count == 1 {
		stmt = "UPDATE heartbeat set status='U' WHERE status='E' AND cluster=? AND secret=?"
		_, err = db.Exec(stmt, cluster, secret)
		if err != nil {
			return err
		}

	} else {
		return err
	}
	return nil
}

func ForgetArbitration(db *sqlx.DB, secret string) error {

	stmt := "DELETE FROM heartbeat WHERE secret=?"
	_, err := db.Exec(stmt, secret)
	if err != nil {
		return err
	}

	return nil
}

func RequestArbitration(db *sqlx.DB, uuid string, secret string, cluster string, main string, uid int, hosts int, failed int) bool {
	log.SetLevel(log.DebugLevel)
	var count int
	tx, err := db.Beginx()
	if err != nil {
		log.Error("(dbhelper.RequestArbitration) Error opening transaction: ", err)
		return false
	}
	// count the number of replication manager Elected that is not me for this cluster
	stmt := "SELECT count(*) FROM heartbeat WHERE cluster=? AND secret=? AND status='E' and uid<>?"
	err = tx.QueryRowx(stmt, cluster, secret, uid).Scan(&count)
	// If none i can consider myself the elected replication-manager
	if err == nil && count == 0 {
		log.Info("No elected managers found for this cluster")
		// A non elected replication-manager may see more nodes than me than in this case lose the election
		stmt = "SELECT count(*) FROM heartbeat WHERE cluster=? AND secret=? AND status = 'U' and uid <> ?  and failed < ?"
		err = tx.QueryRowx(stmt, cluster, secret, uid, failed).Scan(&count)
		if err == nil && count == 0 {
			log.Info("Node won election")
			// stmt = "INSERT INTO heartbeat(secret,uuid,uid,main,date,arbitration_date,cluster, hosts, failed ) VALUES('" + secret + "','" + uuid + "'," + uid + ",'" + main + "', DATETIME('now'), DATETIME('now'),'" + cluster + "'," + hosts + "," + failed + ") ON DUPLICATE KEY UPDATE arbitration_date=DATETIME('now'),date=DATETIME('now'),main='" + main + "',status='E', uuid='" + uuid + "',hosts=" + hosts + ",failed=" + failed
			stmt = `INSERT OR REPLACE INTO heartbeat (secret,uuid,uid,main,date,arbitration_date,cluster,hosts,failed,status)
      VALUES(?,?,?,?,DATETIME('now'),DATETIME('now'),?,?,?,'E')`
			_, err = tx.Exec(stmt, secret, uuid, uid, main, cluster, hosts, failed, secret, cluster, uid)
			if err != nil {
				log.Error("(dbhelper.RequestArbitration) Error executing transaction: ", err)
				tx.Rollback()
				return false
			}
			err = tx.Commit()
			if err != nil {
				log.Error("(dbhelper.RequestArbitration) Error committing transaction: ", err)
				tx.Rollback()
				return false
			}
			return true
		}
		tx.Commit()
		return false
	}
	tx.Commit()
	return false
}

func GetArbitrationMain(db *sqlx.DB, secret string, cluster string) string {
	var main string
	// count the number of replication manager Elected that is not me for this cluster
	stmt := "SELECT main FROM heartbeat WHERE cluster=? AND secret=?  AND status IN ('E')"
	err := db.QueryRowx(stmt, cluster, secret).Scan(&main)
	if err == nil {
		return main
	}
	return ""
}

// SetStatusActiveHeartbeat arbitrator can set or remove election flag "E"
func SetStatusActiveHeartbeat(db *sqlx.DB, uuid string, status string, main string, secret string, uid int) error {

	//stmt := "INSERT INTO heartbeat(secret,uid,main,date ) VALUES('" + secret + "','" + uid + "', DATETIME('now')) ON DUPLICATE KEY UPDATE uuid='" + uuid + "', date=DATETIME('now'),main='" + main + "', status='" + status + "' "
	stmt := `INSERT OR REPLACE INTO heartbeat (secret, uid, main, date)
  VALUES(?,?,?,DATETIME('now'))`
	_, err := db.Exec(stmt, secret, uid, main)
	if err != nil {
		return err
	}
	return err
}
