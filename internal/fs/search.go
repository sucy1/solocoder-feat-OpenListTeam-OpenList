package fs

import (
	"context"
	"regexp"
	"strings"

	"github.com/OpenListTeam/OpenList/v4/internal/conf"
	"github.com/OpenListTeam/OpenList/v4/internal/model"
	"github.com/OpenListTeam/OpenList/v4/internal/op"
)

const maxSearchDepth = 5

type SearchArgs struct {
	Refresh            bool
	NoLog              bool
	WithStorageDetails bool
}

type matcher func(name string) bool

func buildMatcher(query string) (matcher, error) {
	if len(query) >= 2 && strings.HasPrefix(query, "/") && strings.HasSuffix(query, "/") {
		pattern := query[1 : len(query)-1]
		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, err
		}
		return func(name string) bool {
			return re.MatchString(name)
		}, nil
	}
	lowerQuery := strings.ToLower(query)
	return func(name string) bool {
		return strings.Contains(strings.ToLower(name), lowerQuery)
	}, nil
}

func Search(ctx context.Context, reqPath string, query string, args *SearchArgs) ([]model.Obj, error) {
	m, err := buildMatcher(query)
	if err != nil {
		return nil, err
	}

	rootObj, err := Get(ctx, reqPath, &GetArgs{
		NoLog:              args.NoLog,
		WithStorageDetails: args.WithStorageDetails,
	})
	if err != nil {
		return nil, err
	}

	var results []model.Obj

	walkCtx := ctx
	if walkCtx.Value(conf.MetaKey) == nil {
		meta, _ := op.GetNearestMeta(reqPath)
		if meta != nil {
			walkCtx = context.WithValue(ctx, conf.MetaKey, meta)
		}
	}

	err = WalkFS(walkCtx, maxSearchDepth, reqPath, rootObj, func(path string, info model.Obj) error {
		if m(info.GetName()) {
			results = append(results, info)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return results, nil
}
