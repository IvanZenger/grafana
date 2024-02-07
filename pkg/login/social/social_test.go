package social

import (
	"context"
	"testing"

	"github.com/grafana/grafana/pkg/infra/log/logtest"
	"github.com/grafana/grafana/pkg/models/roletype"
	"github.com/grafana/grafana/pkg/services/org"
	"github.com/grafana/grafana/pkg/services/org/orgtest"
	"github.com/grafana/grafana/pkg/util"
	"github.com/stretchr/testify/require"
)

func TestExtractOrgRolesFromRaw(t *testing.T) {
	t.Run("Given a social base", func(t *testing.T) {
		orgService := &orgtest.FakeOrgService{
			ExpectedOrgs: []*org.OrgDTO{
				{ID: 11, Name: "org_foo"},
				{ID: 12, Name: "org_bar"},
				{ID: 13, Name: "org_baz"},
				{ID: 14, Name: "another_org"},
			},
			ExpectedError: org.ErrOrgNotFound,
		}
		ctx := context.TODO()

		tests := []struct {
			name             string
			orgAttributePath string
			orgMapping       string
			rawJSON          string
			expectedError    string
			expectedOrgRoles map[int64]org.RoleType
			WarnLogs         logtest.Logs
		}{
			{
				name:             "Given role claim and valid path, returns valid mapping",
				orgAttributePath: "info.roles",
				orgMapping:       `org_foo:org_foo:Editor org_bar:org_bar:Viewer org_baz:org_baz:Admin`,
				rawJSON:          `{"info": {"roles": ["org_foo", "org_bar","another_org"]}}`,
				expectedOrgRoles: map[int64]roletype.RoleType{11: "Editor", 12: "Viewer"},
			},
			{
				name:             "Given org mapping with wildcard and valid path, returns valid mapping",
				orgAttributePath: "info.roles",
				orgMapping:       `*:org_foo:Editor org_bar:org_bar:Viewer org_baz:org_baz:Admin`,
				rawJSON:          `{"info": {"roles": ["org_bar","another_org"]}}`,
				expectedOrgRoles: map[int64]roletype.RoleType{11: "Editor", 12: "Viewer"},
			},
			{
				name:             "Give several mapping configuration for one unit, last wins",
				orgAttributePath: "info.roles",
				orgMapping:       `org_foo:org_foo:Editor org_foo:org_foo:Viewer org_baz:org_baz:Admin`,
				rawJSON:          `{"info": {"roles": ["org_foo", "org_baz"]}}`,
				expectedOrgRoles: map[int64]roletype.RoleType{11: "Viewer", 13: "Admin"},
			},
			{
				name:             "Given invalid JMES path, returns an error",
				orgAttributePath: "[",
				orgMapping:       ``,
				rawJSON:          `{}`,
				expectedError:    `failed to search user info JSON response with provided path: "[": SyntaxError: Incomplete expression`,
			},
			{
				name:             "With invalid role, returns an error",
				orgAttributePath: "info.roles",
				orgMapping:       `org_foo:org_foo:Editor  org_bar:org_bar:invalid_role `,
				rawJSON:          `{"info": {"roles": ["org_foo", "org_bar"]}}`,
				expectedError:    "invalid role type: invalid_role",
			},
			{
				name:             "With unfound org, skip and continue",
				orgAttributePath: "info.roles",
				orgMapping:       `org_foo:invalid_org:Editor  org_bar:org_bar:Viewer`,
				rawJSON:          `{"info": {"roles": ["org_foo","org_bar"]}}`,
				expectedOrgRoles: map[int64]roletype.RoleType{12: "Viewer"},
				WarnLogs: logtest.Logs{
					Calls:   1,
					Message: "Unknown organization. Skipping.",
					Ctx:     []interface{}{"config_option", "info.roles", "mapping", "{org_foo 0 invalid_org Editor}"}},
			},
		}

		for _, test := range tests {
			logger := &logtest.Fake{}
			s := SocialBase{log: logger, orgService: orgService}
			s.orgAttributePath = test.orgAttributePath
			s.orgMapping = util.SplitString(test.orgMapping)

			t.Run(test.name, func(t *testing.T) {
				orgRoles, err := s.extractOrgRolesFromRaw(ctx, []byte(test.rawJSON))
				if test.expectedError == "" {
					require.NoError(t, err, "Testing case %q", test.name)
				} else {
					require.EqualError(t, err, test.expectedError, "Testing case %q", test.name)
				}
				require.Equal(t, test.expectedOrgRoles, orgRoles)
				require.Equal(t, test.WarnLogs, logger.WarnLogs)
			})
		}
	})
}
