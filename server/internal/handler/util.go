package handler

import (
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/collabos/collabos/server/internal/util"
)

func uuidStr(u pgtype.UUID) string { return util.UUIDString(u) }
