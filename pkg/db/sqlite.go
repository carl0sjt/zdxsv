package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"github.com/pkg/errors"
)

type SQLiteDB struct {
	*sqlx.DB
	*SQLiteCache
}

type SQLiteCache struct {
	mtx          sync.Mutex
	rankingCache map[string][]*RankingRecord
}

func NewSQLiteCache() *SQLiteCache {
	return &SQLiteCache{
		rankingCache: map[string][]*RankingRecord{},
	}
}

func (c *SQLiteCache) deleteRankingCache() {
	c.mtx.Lock()
	c.rankingCache = map[string][]*RankingRecord{}
	c.mtx.Unlock()
}

const schema = `
CREATE TABLE IF NOT EXISTS account (
	login_key text,
	session_id text default '',
	last_user_id text default '',
	created_ip text default '',
	created timestamp,
	last_login timestamp,
	system integer default 0,
	PRIMARY KEY (login_key)
);
CREATE TABLE IF NOT EXISTS user (
	user_id text,
	login_key text,
	session_id text default '',
	name text default 'default',
	team text default '',
	battle_count integer default 0,
	win_count integer default 0,
	lose_count integer default 0,
	kill_count integer default 0,
	death_count integer default 0,
	aeug_battle_count integer default 0,
	aeug_win_count integer default 0,
	aeug_lose_count integer default 0,
	aeug_kill_count integer default 0,
	aeug_death_count integer default 0,
	titans_battle_count integer default 0,
	titans_win_count integer default 0,
	titans_lose_count integer default 0,
	titans_kill_count integer default 0,
	titans_death_count integer default 0,
	daily_battle_count integer default 0,
	daily_win_count integer default 0,
	daily_lose_count integer default 0,
	created timestamp,
	system integer default 0,
	PRIMARY KEY (user_id, login_key)
);
CREATE TABLE IF NOT EXISTS battle_record (
	battle_code text,
	user_id     text,
	user_name 	text,
	pilot_name 	text,
	players     integer default 0,
	aggregate   integer default 0,
	pos         integer default 0,
	side        integer default 0,
	round       integer default 0,
	win         integer default 0,
	lose        integer default 0,
	kill        integer default 0,
	death       integer default 0,
	frame       integer default 0,
	result      text default '',
	created     timestamp,
	updated     timestamp,
	system      integer default 0,
	PRIMARY KEY (battle_code, user_id)
);
`

const indexes = `
CREATE INDEX IF NOT EXISTS BATTLE_RECORD_USER_ID ON battle_record(user_id);
CREATE INDEX IF NOT EXISTS BATTLE_RECORD_PLAYERS ON battle_record(players);
CREATE INDEX IF NOT EXISTS BATTLE_RECORD_CREATED ON battle_record(created);
CREATE INDEX IF NOT EXISTS BATTLE_RECORD_AGGRIGATE ON battle_record(aggregate);
`

func (db SQLiteDB) Init() error {
	_, err := db.Exec(schema + indexes)
	return err
}

func (db SQLiteDB) Migrate() error {
	ctx := context.Background()
	tables := []string{"account", "user", "battle_record"}

	// begin tx
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelDefault})
	if err != nil {
		return errors.Wrap(err, "Begin failed")
	}

	// create table if not exists
	_, err = tx.Exec(schema)
	if err != nil {
		tx.Rollback()
		return errors.Wrap(err, "Begin failed")
	}

	// copy all tables
	for _, table := range tables {
		tmp := table + "_tmp"
		_, err = tx.Exec(`ALTER TABLE ` + table + ` RENAME TO ` + tmp)
		if err != nil {
			tx.Rollback()
			return errors.Wrap(err, "ALTER TABLE failed")
		}
	}

	// create new table
	_, err = tx.Exec(schema + indexes)
	if err != nil {
		tx.Rollback()
		return errors.Wrap(err, "failed to create new tables")
	}

	// copy old table into new table and drop old table
	// it works unless key name is changed
	for _, table := range tables {
		tmp := table + "_tmp"
		rows, err := tx.Query(`SELECT * FROM ` + tmp + ` LIMIT 1`)
		if err != nil {
			tx.Rollback()
			return errors.Wrap(err, "SELECT failed")
		}

		columns, err := rows.Columns()
		if err != nil {
			tx.Rollback()
			return errors.Wrap(err, "Columns() failed")
		}
		rows.Close()

		_, err = tx.Exec(`INSERT INTO ` + table + `(` + strings.Join(columns, ",") + `) SELECT * FROM ` + tmp)
		if err != nil {
			tx.Rollback()
			return errors.Wrap(err, "INSERT failed")
		}

		_, err = tx.Exec(`DROP TABLE ` + tmp)
		if err != nil {
			tx.Rollback()
			return errors.Wrap(err, "DROP TABLE failed")
		}
	}

	return tx.Commit()
}

func (db SQLiteDB) RegisterAccount(ip string) (*Account, error) {
	key := genLoginKey()
	now := time.Now()
	_, err := db.Exec(`
INSERT INTO account
	(login_key, created_ip, created, last_login, system)
VALUES
	(?, ?, ?, ?, ?)`, key, ip, now, now, 0)
	if err != nil {
		return nil, err
	}
	a := &Account{
		LoginKey:  key,
		CreatedIP: ip,
	}
	return a, nil
}

func (db SQLiteDB) RegisterAccountWithLoginKey(ip string, loginKey string) (*Account, error) {
	now := time.Now()
	_, err := db.Exec(`
INSERT INTO account
	(login_key, created_ip, created, last_login, system)
VALUES
	(?, ?, ?, ?, ?)`, loginKey, ip, now, now, 0)
	if err != nil {
		return nil, err
	}
	a := &Account{
		LoginKey:  loginKey,
		CreatedIP: ip,
	}
	return a, nil
}

func (db SQLiteDB) GetAccountByLoginKey(key string) (*Account, error) {
	a := &Account{}
	err := db.QueryRowx("SELECT * FROM account WHERE login_key = ?", key).StructScan(a)
	if err != nil {
		return nil, err
	}
	return a, nil
}

func (db SQLiteDB) LoginAccount(a *Account) error {
	a.SessionID = genSessionID()
	a.LastLogin = time.Now()
	_, err := db.Exec(`
UPDATE
	account
SET
	session_id = ?,
	last_login = ?
WHERE
	login_key = ?`,
		a.SessionID,
		a.LastLogin,
		a.LoginKey)
	return err
}

func (db SQLiteDB) RegisterUser(loginKey string) (*User, error) {
	userID := genUserID()
	now := time.Now()
	_, err := db.Exec(`INSERT INTO user (user_id, login_key, created) VALUES (?, ?, ?)`, userID, loginKey, now)
	if err != nil {
		return nil, err
	}
	u := &User{
		LoginKey: loginKey,
		UserID:   userID,
		Created:  now,
	}
	return u, nil
}

func (db SQLiteDB) GetUserList(loginKey string) ([]*User, error) {
	rows, err := db.Queryx(`SELECT * FROM user WHERE login_key = ?`, loginKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	users := []*User{}
	for rows.Next() {
		u := new(User)
		err = rows.StructScan(u)
		if err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, nil
}

func (db SQLiteDB) GetUser(userID string) (*User, error) {
	u := &User{}
	err := db.Get(u, `SELECT * FROM user WHERE user_id = ?`, userID)
	return u, err
}

func (db SQLiteDB) LoginUser(user *User) error {
	a, err := db.GetAccountByLoginKey(user.LoginKey)
	if err != nil {
		return err
	}
	a.LastUserID = user.UserID

	_, err = db.Exec(`UPDATE account SET last_user_id = ? WHERE login_key = ?`, a.LastUserID, a.LoginKey)
	if err != nil {
		return err
	}

	_, err = db.Exec(`UPDATE user SET session_id = ? WHERE user_id = ?`, user.SessionID, user.UserID)
	return err
}

func (db SQLiteDB) UpdateUser(user *User) error {
	_, err := db.NamedExec(`
UPDATE user
SET
	name = :name,
	team = :team,
	battle_count = :battle_count,
	win_count = :win_count,
	lose_count = :lose_count,
	kill_count = :kill_count,
	death_count = :death_count,
	aeug_battle_count = :aeug_battle_count,
	aeug_win_count = :aeug_win_count,
	aeug_lose_count = :aeug_lose_count,
	aeug_kill_count = :aeug_kill_count,
	aeug_death_count = :aeug_death_count,
	titans_battle_count = :titans_battle_count,
	titans_win_count = :titans_win_count,
	titans_lose_count = :titans_lose_count,
	titans_kill_count = :titans_kill_count,
	titans_death_count = :titans_death_count,
	daily_battle_count = :daily_battle_count,
	daily_win_count = :daily_win_count,
	daily_lose_count = :daily_lose_count,
	system = :system
WHERE
	user_id = :user_id`, user)
	return err
}

func (db SQLiteDB) AddBattleRecord(battleRecord *BattleRecord) error {
	now := time.Now()
	battleRecord.Updated = now
	battleRecord.Created = now
	_, err := db.NamedExec(`
INSERT INTO battle_record
	(battle_code, user_id, user_name, pilot_name, players, aggregate, pos, side, created, updated, system)
VALUES
	(:battle_code, :user_id, :user_name, :pilot_name, :players, :aggregate, :pos, :side, :created, :updated, :system)`,
		battleRecord)
	return err
}

func (db SQLiteDB) UpdateBattleRecord(battle *BattleRecord) error {
	battle.Updated = time.Now()
	_, err := db.NamedExec(`
UPDATE battle_record
SET
	round = :round,
	win = :win,
	lose = :lose,
	kill = :kill,
	death = :death,
	frame = :frame,
	result = :result,
	updated = :updated,
	system = :system
WHERE
	battle_code = :battle_code AND user_id = :user_id`, battle)

	if err == nil && battle.Aggregate != 0 {
		// refresh rakning page
		db.deleteRankingCache()
	}
	return err
}

func (db SQLiteDB) GetBattleRecordUser(battleCode string, userID string) (*BattleRecord, error) {
	b := new(BattleRecord)
	err := db.Get(b, `SELECT * FROM battle_record WHERE battle_code = ? AND user_id = ?`, battleCode, userID)
	return b, err
}

func (db SQLiteDB) CalculateUserTotalBattleCount(userID string, side byte) (ret BattleCountResult, err error) {
	if side == 0 {
		r := db.QueryRow(`
			SELECT TOTAL(round), TOTAL(win), TOTAL(lose), TOTAL(kill), TOTAL(death) FROM battle_record
			WHERE user_id = ? AND aggregate <> 0 AND players = 4`, userID)
		err = r.Scan(&ret.Battle, &ret.Win, &ret.Lose, &ret.Kill, &ret.Death)
		return
	}
	r := db.QueryRow(`
		SELECT TOTAL(round), TOTAL(win), TOTAL(lose), TOTAL(kill), TOTAL(death) FROM battle_record
		WHERE user_id = ? AND aggregate <> 0 AND players = 4 AND side = ?`, userID, side)
	err = r.Scan(&ret.Battle, &ret.Win, &ret.Lose, &ret.Kill, &ret.Death)
	return
}

func (db SQLiteDB) CalculateUserDailyBattleCount(userID string) (ret BattleCountResult, err error) {
	r := db.QueryRow(`
		SELECT TOTAL(round), TOTAL(win), TOTAL(lose), TOTAL(kill), TOTAL(death) FROM battle_record
		WHERE user_id = ? AND aggregate <> 0 AND players = 4 AND created > ?`,
		userID, time.Now().AddDate(0, 0, -1))
	err = r.Scan(&ret.Battle, &ret.Win, &ret.Lose, &ret.Kill, &ret.Death)
	return
}

func (db SQLiteDB) GetWinCountRanking(side byte) ([]*RankingRecord, error) {
	cacheKey := fmt.Sprint("win", side)
	db.mtx.Lock()
	ranking, ok := db.rankingCache[cacheKey]
	db.mtx.Unlock()
	if ok {
		return ranking, nil
	}

	var rows *sqlx.Rows
	var err error

	target := "win_count"
	if side == 1 {
		target = "aeug_win_count"
	} else if side == 2 {
		target = "titans_win_count"
	}

	rows, err = db.Queryx(`
		SELECT RANK() OVER(ORDER BY `+target+` DESC) as rank,
		user_id, name, team,
		battle_count, win_count, lose_count, kill_count, death_count,
		aeug_battle_count, aeug_win_count, aeug_lose_count, aeug_kill_count, aeug_death_count,
		titans_battle_count, titans_win_count, titans_lose_count, titans_kill_count, titans_death_count
		FROM user ORDER BY rank LIMIT ?`, 100)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ranking = []*RankingRecord{}
	for rows.Next() {
		u := new(RankingRecord)
		err = rows.StructScan(u)
		if err != nil {
			return nil, err
		}
		if !utf8.ValidString(u.Name) {
			u.Name = "？"
		}
		if !utf8.ValidString(u.Team) {
			u.Team = "？"
		}
		ranking = append(ranking, u)
	}

	db.mtx.Lock()
	db.rankingCache[cacheKey] = ranking
	db.mtx.Unlock()

	return ranking, nil
}

func (db SQLiteDB) GetKillCountRanking(side byte) ([]*RankingRecord, error) {
	cacheKey := fmt.Sprint("kill", side)
	db.mtx.Lock()
	ranking, ok := db.rankingCache[cacheKey]
	db.mtx.Unlock()
	if ok {
		return ranking, nil
	}

	var rows *sqlx.Rows
	var err error

	target := "kill_count"
	if side == 1 {
		target = "aeug_kill_count"
	} else if side == 2 {
		target = "titans_kill_count"
	}

	rows, err = db.Queryx(`
		SELECT RANK() OVER(ORDER BY `+target+` DESC) as rank,
		user_id, name, team,
		battle_count, win_count, lose_count, kill_count, death_count,
		aeug_battle_count, aeug_win_count, aeug_lose_count, aeug_kill_count, aeug_death_count,
		titans_battle_count, titans_win_count, titans_lose_count, titans_kill_count, titans_death_count
		FROM user ORDER BY rank LIMIT ?`, 100)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ranking = []*RankingRecord{}
	for rows.Next() {
		u := new(RankingRecord)
		err = rows.StructScan(u)
		if err != nil {
			return nil, err
		}
		if !utf8.ValidString(u.Name) {
			u.Name = "？"
		}
		if !utf8.ValidString(u.Team) {
			u.Team = "？"
		}
		ranking = append(ranking, u)
	}

	db.mtx.Lock()
	db.rankingCache[cacheKey] = ranking
	db.mtx.Unlock()

	return ranking, nil
}
