package locationidentity

import (
	"context"
	"database/sql"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"

	"athlix/backend/internal/platform/db"
)

// ImportResult is what ImportDistricts did. SkippedLines lists every line that was not written.
type ImportResult struct {
	Inserted     int           `json:"inserted"`
	Updated      int           `json:"updated"`
	Skipped      int           `json:"skipped"`
	SkippedLines []SkippedLine `json:"skippedLines"`
}

// SkippedLine is one CSV line that was not imported (1-based line number).
type SkippedLine struct {
	Line   int    `json:"line"`
	Reason string `json:"reason"`
}

type importLine struct {
	line      int
	stateCode string
	name      string
	lgd       sql.NullInt64
	aliases   []string
}

// ImportDistricts upserts districts from a CSV file (cmd/admincli import-districts).
//
// Columns: state_code,district_name,lgd_code[,aliases]. aliases is ';'-separated. A header row
// is allowed. A row matches an existing district by lgd_code, else by (state, name); a match is
// updated (lgd_code / aliases are kept when the file leaves them empty), otherwise inserted.
// Unknown state codes and bad rows are skipped with their line number. Everything runs in one
// transaction: a database error rolls the whole import back.
func ImportDistricts(ctx context.Context, conn *sql.DB, r io.Reader) (ImportResult, error) {
	res := ImportResult{SkippedLines: []SkippedLine{}}
	skip := func(line int, reason string) {
		res.Skipped++
		res.SkippedLines = append(res.SkippedLines, SkippedLine{Line: line, Reason: reason})
	}
	lines, err := readImportLines(r, skip)
	if err != nil {
		return res, err
	}
	err = db.WithTx(ctx, conn, func(tx *sql.Tx) error {
		states, err := stateCodes(ctx, tx)
		if err != nil {
			return err
		}
		for _, l := range lines {
			stateID, ok := states[l.stateCode]
			if !ok {
				skip(l.line, "unknown state code")
				continue
			}
			id, err := findDistrict(ctx, tx, stateID, l)
			if err != nil {
				return err
			}
			if id > 0 {
				if _, err := tx.ExecContext(ctx, `UPDATE districts SET state_id = ?, name = ?, lgd_code = COALESCE(?, lgd_code),
					aliases = COALESCE(?, aliases) WHERE id = ?`, stateID, l.name, nullInt(l.lgd), aliasesArg(l.aliases), id); err != nil {
					if db.IsDuplicate(err) {
						skip(l.line, "conflicts with another district (name or lgd_code)")
						continue
					}
					return fmt.Errorf("line %d: %w", l.line, err)
				}
				res.Updated++
				continue
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO districts (state_id, name, lgd_code, aliases, active) VALUES (?, ?, ?, ?, 1)`,
				stateID, l.name, nullInt(l.lgd), aliasesArg(l.aliases)); err != nil {
				if db.IsDuplicate(err) {
					skip(l.line, "conflicts with another district (name or lgd_code)")
					continue
				}
				return fmt.Errorf("line %d: %w", l.line, err)
			}
			res.Inserted++
		}
		return nil
	})
	if err != nil {
		return ImportResult{SkippedLines: []SkippedLine{}}, err
	}
	return res, nil
}

func readImportLines(r io.Reader, skip func(int, string)) ([]importLine, error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1
	cr.TrimLeadingSpace = true
	var out []importLine
	for first := true; ; first = false {
		rec, err := cr.Read()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		var pe *csv.ParseError
		if errors.As(err, &pe) {
			skip(pe.StartLine, "malformed CSV")
			continue
		}
		if err != nil {
			return nil, err
		}
		line, _ := cr.FieldPos(0)
		for i := range rec {
			rec[i] = strings.TrimSpace(rec[i])
		}
		if first && len(rec) > 0 && strings.EqualFold(strings.TrimPrefix(rec[0], "\uFEFF"), "state_code") {
			continue // header row
		}
		if first && len(rec) > 0 {
			rec[0] = strings.TrimPrefix(rec[0], "\uFEFF")
		}
		if len(rec) < 3 || len(rec) > 4 {
			skip(line, "expected 3 or 4 columns")
			continue
		}
		l := importLine{line: line, stateCode: strings.ToUpper(rec[0]), name: rec[1]}
		if l.stateCode == "" {
			skip(line, "state_code is required")
			continue
		}
		if l.name == "" || utf8.RuneCountInString(l.name) > maxDistrictName {
			skip(line, "district_name is required (at most 80 characters)")
			continue
		}
		if rec[2] != "" {
			n, err := strconv.ParseInt(rec[2], 10, 64)
			if err != nil || n <= 0 {
				skip(line, "lgd_code must be a positive number")
				continue
			}
			l.lgd = sql.NullInt64{Int64: n, Valid: true}
		}
		if len(rec) == 4 {
			bad := map[string]string{}
			l.aliases = cleanAliases(strings.Split(rec[3], ";"), bad)
			if len(bad) > 0 {
				skip(line, bad["aliases"])
				continue
			}
		}
		out = append(out, l)
	}
}

func stateCodes(ctx context.Context, q db.Querier) (map[string]int64, error) {
	rows, err := q.QueryContext(ctx, `SELECT id, code FROM states`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		var id int64
		var code string
		if err := rows.Scan(&id, &code); err != nil {
			return nil, err
		}
		out[strings.ToUpper(code)] = id
	}
	return out, rows.Err()
}

// findDistrict returns the id of the district a line updates (0 = insert).
func findDistrict(ctx context.Context, q db.Querier, stateID int64, l importLine) (int64, error) {
	var id int64
	if l.lgd.Valid {
		err := q.QueryRowContext(ctx, `SELECT id FROM districts WHERE lgd_code = ?`, l.lgd.Int64).Scan(&id)
		if err == nil {
			return id, nil
		}
		if !db.IsNoRows(err) {
			return 0, err
		}
	}
	err := q.QueryRowContext(ctx, `SELECT id FROM districts WHERE state_id = ? AND name = ?`, stateID, l.name).Scan(&id)
	if db.IsNoRows(err) {
		return 0, nil
	}
	return id, err
}
