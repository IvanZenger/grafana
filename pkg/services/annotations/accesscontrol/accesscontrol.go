package accesscontrol

import (
	"context"

	"github.com/grafana/grafana/pkg/infra/db"
	ac "github.com/grafana/grafana/pkg/services/accesscontrol"
	"github.com/grafana/grafana/pkg/services/annotations"
	"github.com/grafana/grafana/pkg/services/auth/identity"
	"github.com/grafana/grafana/pkg/services/dashboards/dashboardaccess"
	"github.com/grafana/grafana/pkg/services/featuremgmt"
	"github.com/grafana/grafana/pkg/services/sqlstore/permissions"
	"github.com/grafana/grafana/pkg/services/sqlstore/searchstore"
	"github.com/grafana/grafana/pkg/util/errutil"
)

var (
	ErrReadForbidden = errutil.NewBase(
		errutil.StatusForbidden,
		"annotations.accesscontrol.read",
		errutil.WithPublicMessage("User missing permissions"),
	)
	ErrAccessControlInternal = errutil.NewBase(
		errutil.StatusInternal,
		"annotations.accesscontrol.internal",
		errutil.WithPublicMessage("Internal error while checking permissions"),
	)
)

type AuthService struct {
	db       db.DB
	features featuremgmt.FeatureToggles
}

func NewAuthService(db db.DB, features featuremgmt.FeatureToggles) *AuthService {
	return &AuthService{
		db:       db,
		features: features,
	}
}

// Authorize checks if the user has permission to read annotations, then returns a struct containing dashboards and scope types that the user has access to.
func (authz *AuthService) Authorize(ctx context.Context, orgID int64, user identity.Requester) (*AccessResources, error) {
	if user == nil || user.IsNil() {
		return nil, ErrReadForbidden.Errorf("missing user")
	}

	scopes, has := user.GetPermissions()[ac.ActionAnnotationsRead]
	if !has {
		return nil, ErrReadForbidden.Errorf("user does not have permission to read annotations")
	}

	scopeTypes := annotationScopeTypes(scopes)

	var visibleDashboards map[string]int64
	var err error
	if _, ok := scopeTypes[annotations.Dashboard.String()]; ok {
		visibleDashboards, err = authz.userVisibleDashboards(ctx, user, orgID)
		if err != nil {
			return nil, ErrAccessControlInternal.Errorf("failed to fetch dashboards: %w", err)
		}
	}

	return &AccessResources{
		Dashboards: visibleDashboards,
		ScopeTypes: scopeTypes,
	}, nil
}

func (authz *AuthService) userVisibleDashboards(ctx context.Context, user identity.Requester, orgID int64) (map[string]int64, error) {
	recursiveQueriesSupported, err := authz.db.RecursiveQueriesAreSupported()
	if err != nil {
		return nil, err
	}

	filters := []any{
		permissions.NewAccessControlDashboardPermissionFilter(user, dashboardaccess.PERMISSION_VIEW, searchstore.TypeDashboard, authz.features, recursiveQueriesSupported),
		searchstore.OrgFilter{OrgId: orgID},
	}

	sb := &searchstore.Builder{Dialect: authz.db.GetDialect(), Filters: filters, Features: authz.features}

	visibleDashboards := make(map[string]int64)

	var page int64 = 1
	var limit int64 = 1000
	for {
		var res []dashboardProjection
		sql, params := sb.ToSQL(limit, page)

		err = authz.db.WithDbSession(ctx, func(sess *db.Session) error {
			return sess.SQL(sql, params...).Find(&res)
		})
		if err != nil {
			return nil, err
		}

		for _, p := range res {
			visibleDashboards[p.UID] = p.ID
		}

		// if the result is less than the limit, we have reached the end
		if len(res) < int(limit) {
			break
		}

		page++
	}

	return visibleDashboards, nil
}

func annotationScopeTypes(scopes []string) map[any]struct{} {
	allScopeTypes := map[any]struct{}{
		annotations.Dashboard.String():    {},
		annotations.Organization.String(): {},
	}

	types, hasWildcardScope := ac.ParseScopes(ac.ScopeAnnotationsProvider.GetResourceScopeType(""), scopes)
	if hasWildcardScope {
		types = allScopeTypes
	}

	return types
}
