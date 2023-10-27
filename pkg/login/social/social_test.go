package social

import (
	"context"
	"testing"

	"github.com/grafana/grafana/pkg/infra/log/logtest"
	"github.com/grafana/grafana/pkg/models/roletype"
	"github.com/grafana/grafana/pkg/services/org"
	"github.com/grafana/grafana/pkg/services/org/orgtest"
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
			name                  string
			orgRolesAttributePath string
			rawJSON               string
			expectedError         string
			expectedOrgRoles      map[int64]org.RoleType
			WarnLogs              logtest.Logs
		}{
			{
				name:                  "Given role claim and valid JMES path, returns valid mapping",
				orgRolesAttributePath: `info.roles[?starts_with(@, 'org_')][{"OrgName": @, "Role": 'Editor'}][]`,
				rawJSON:               `{"info": {"roles": ["org_foo", "org_bar","another_org"]}}`,
				expectedOrgRoles:      map[int64]roletype.RoleType{11: "Editor", 12: "Editor"},
			},
			{
				name:                  "Given several role claims and valid JMES path, last wins",
				orgRolesAttributePath: `[viewerGroups[*][{"OrgName": @, "Role": 'Viewer'}], editorGroups[*][{"OrgName": @, "Role": 'Editor'}]][][]`,
				rawJSON:               `{"viewerGroups": ["org_foo", "org_bar"], "editorGroups": ["org_bar", "org_baz"]}`,
				expectedOrgRoles:      map[int64]roletype.RoleType{11: "Viewer", 12: "Editor", 13: "Editor"},
			},
			{
				name:                  "Given invalid JMES path, returns an error",
				orgRolesAttributePath: `[`,
				rawJSON:               `{}`,
				expectedError:         `failed to search user info JSON response with provided path: "[": SyntaxError: Incomplete expression`,
			},
			{
				name:                  "With invalid role, returns an error",
				orgRolesAttributePath: `[{"OrgName": 'org_foo', "Role": 'invalid role'}]`,
				rawJSON:               `{}`,
				expectedError:         "invalid role value: Invalid Role",
			},
			{
				name:                  "With unfound org, skip and continue",
				orgRolesAttributePath: `[{"OrgName": 'invalid org', "Role": 'Editor'}, {"OrgName": 'org_bar', "Role": 'Viewer'}]`,
				rawJSON:               `{}`,
				expectedOrgRoles:      map[int64]roletype.RoleType{12: "Viewer"},
				WarnLogs: logtest.Logs{
					Calls:   1,
					Message: "Unknown organization. Skipping.",
					Ctx:     []interface{}{"config_option", "[{\"OrgName\": 'invalid org', \"Role\": 'Editor'}, {\"OrgName\": 'org_bar', \"Role\": 'Viewer'}]", "mapping", "{0 invalid org Editor}"}},
			},
			{
				name:                  "With invalid org id, skip and continue",
				orgRolesAttributePath: `[{"OrgId": '-1', "Role": 'Editor'}, {"OrgName": 'org_bar', "Role": 'Viewer'}]`,
				rawJSON:               `{}`,
				expectedOrgRoles:      map[int64]roletype.RoleType{12: "Viewer"},
				WarnLogs: logtest.Logs{
					Calls:   1,
					Message: "Incorrect mapping found. Skipping.",
					Ctx:     []interface{}{"config_option", "[{\"OrgId\": '-1', \"Role\": 'Editor'}, {\"OrgName\": 'org_bar', \"Role\": 'Viewer'}]", "mapping", "{-1  Editor}"}},
			},
		}

		for _, test := range tests {
			logger := &logtest.Fake{}
			s := SocialBase{log: logger, orgService: orgService}
			s.orgRolesAttributePath = test.orgRolesAttributePath
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
