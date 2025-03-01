package ldaputil

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
	"text/template"

	"github.com/hashicorp/go-secure-stdlib/tlsutil"
	"github.com/hashicorp/vault/sdk/framework"

	"github.com/hashicorp/errwrap"
)

// ConfigFields returns all the config fields that can potentially be used by the LDAP client.
// Not all fields will be used by every integration.
func ConfigFields() map[string]*framework.FieldSchema {
	return map[string]*framework.FieldSchema{
		"anonymous_group_search": {
			Type:        framework.TypeBool,
			Default:     false,
			Description: "Use anonymous binds when performing LDAP group searches (if true the initial credentials will still be used for the initial connection test).",
			DisplayAttrs: &framework.DisplayAttributes{
				Name: "Anonymous group search",
			},
		},
		"url": {
			Type:        framework.TypeString,
			Default:     "ldap://127.0.0.1",
			Description: "LDAP URL to connect to (default: ldap://127.0.0.1). Multiple URLs can be specified by concatenating them with commas; they will be tried in-order.",
			DisplayAttrs: &framework.DisplayAttributes{
				Name: "URL",
			},
		},

		"userdn": {
			Type:        framework.TypeString,
			Description: "LDAP domain to use for users (eg: ou=People,dc=example,dc=org)",
			DisplayAttrs: &framework.DisplayAttributes{
				Name: "User DN",
			},
		},

		"binddn": {
			Type:        framework.TypeString,
			Description: "LDAP DN for searching for the user DN (optional)",
			DisplayAttrs: &framework.DisplayAttributes{
				Name: "Name of Object to bind (binddn)",
			},
		},

		"bindpass": {
			Type:        framework.TypeString,
			Description: "LDAP password for searching for the user DN (optional)",
			DisplayAttrs: &framework.DisplayAttributes{
				Sensitive: true,
			},
		},

		"groupdn": {
			Type:        framework.TypeString,
			Description: "LDAP search base to use for group membership search (eg: ou=Groups,dc=example,dc=org)",
			DisplayAttrs: &framework.DisplayAttributes{
				Name: "Group DN",
			},
		},

		"groupfilter": {
			Type:    framework.TypeString,
			Default: "(|(memberUid={{.Username}})(member={{.UserDN}})(uniqueMember={{.UserDN}}))",
			Description: `Go template for querying group membership of user (optional)
The template can access the following context variables: UserDN, Username
Example: (&(objectClass=group)(member:1.2.840.113556.1.4.1941:={{.UserDN}}))
Default: (|(memberUid={{.Username}})(member={{.UserDN}})(uniqueMember={{.UserDN}}))`,
			DisplayAttrs: &framework.DisplayAttributes{
				Name: "Group Filter",
			},
		},

		"groupattr": {
			Type:    framework.TypeString,
			Default: "cn",
			Description: `LDAP attribute to follow on objects returned by <groupfilter>
in order to enumerate user group membership.
Examples: "cn" or "memberOf", etc.
Default: cn`,
			DisplayAttrs: &framework.DisplayAttributes{
				Name:  "Group Attribute",
				Value: "cn",
			},
		},

		"upndomain": {
			Type:        framework.TypeString,
			Description: "Enables userPrincipalDomain login with [username]@UPNDomain (optional)",
			DisplayAttrs: &framework.DisplayAttributes{
				Name: "User Principal (UPN) Domain",
			},
		},

		"userattr": {
			Type:        framework.TypeString,
			Default:     "cn",
			Description: "Attribute used for users (default: cn)",
			DisplayAttrs: &framework.DisplayAttributes{
				Name:  "User Attribute",
				Value: "cn",
			},
		},

		"certificate": {
			Type:        framework.TypeString,
			Description: "CA certificate to use when verifying LDAP server certificate, must be x509 PEM encoded (optional)",
			DisplayAttrs: &framework.DisplayAttributes{
				Name:     "CA certificate",
				EditType: "file",
			},
		},

		"client_tls_cert": {
			Type:        framework.TypeString,
			Description: "Client certificate to provide to the LDAP server, must be x509 PEM encoded (optional)",
			DisplayAttrs: &framework.DisplayAttributes{
				Name:     "Client certificate",
				EditType: "file",
			},
		},

		"client_tls_key": {
			Type:        framework.TypeString,
			Description: "Client certificate key to provide to the LDAP server, must be x509 PEM encoded (optional)",
			DisplayAttrs: &framework.DisplayAttributes{
				Name:     "Client key",
				EditType: "file",
			},
		},

		"discoverdn": {
			Type:        framework.TypeBool,
			Description: "Use anonymous bind to discover the bind DN of a user (optional)",
			DisplayAttrs: &framework.DisplayAttributes{
				Name: "Discover DN",
			},
		},

		"insecure_tls": {
			Type:        framework.TypeBool,
			Description: "Skip LDAP server SSL Certificate verification - VERY insecure (optional)",
			DisplayAttrs: &framework.DisplayAttributes{
				Name: "Insecure TLS",
			},
		},

		"starttls": {
			Type:        framework.TypeBool,
			Description: "Issue a StartTLS command after establishing unencrypted connection (optional)",
			DisplayAttrs: &framework.DisplayAttributes{
				Name: "Issue StartTLS",
			},
		},

		"tls_min_version": {
			Type:        framework.TypeString,
			Default:     "tls12",
			Description: "Minimum TLS version to use. Accepted values are 'tls10', 'tls11', 'tls12' or 'tls13'. Defaults to 'tls12'",
			DisplayAttrs: &framework.DisplayAttributes{
				Name: "Minimum TLS Version",
			},
			AllowedValues: []interface{}{"tls10", "tls11", "tls12", "tls13"},
		},

		"tls_max_version": {
			Type:        framework.TypeString,
			Default:     "tls12",
			Description: "Maximum TLS version to use. Accepted values are 'tls10', 'tls11', 'tls12' or 'tls13'. Defaults to 'tls12'",
			DisplayAttrs: &framework.DisplayAttributes{
				Name: "Maximum TLS Version",
			},
			AllowedValues: []interface{}{"tls10", "tls11", "tls12", "tls13"},
		},

		"deny_null_bind": {
			Type:        framework.TypeBool,
			Default:     true,
			Description: "Denies an unauthenticated LDAP bind request if the user's password is empty; defaults to true",
		},

		"case_sensitive_names": {
			Type:        framework.TypeBool,
			Description: "If true, case sensitivity will be used when comparing usernames and groups for matching policies.",
		},

		"use_token_groups": {
			Type:        framework.TypeBool,
			Default:     false,
			Description: "If true, use the Active Directory tokenGroups constructed attribute of the user to find the group memberships. This will find all security groups including nested ones.",
		},

		"use_pre111_group_cn_behavior": {
			Type:        framework.TypeBool,
			Description: "In Vault 1.1.1 a fix for handling group CN values of different cases unfortunately introduced a regression that could cause previously defined groups to not be found due to a change in the resulting name. If set true, the pre-1.1.1 behavior for matching group CNs will be used. This is only needed in some upgrade scenarios for backwards compatibility. It is enabled by default if the config is upgraded but disabled by default on new configurations.",
		},

		"request_timeout": {
			Type:        framework.TypeDurationSecond,
			Description: "Timeout, in seconds, for the connection when making requests against the server before returning back an error.",
			Default:     "90s",
		},
	}
}

/*
 * Creates and initializes a ConfigEntry object with its default values,
 * as specified by the passed schema.
 */
func NewConfigEntry(existing *ConfigEntry, d *framework.FieldData) (*ConfigEntry, error) {
	var hadExisting bool
	var cfg *ConfigEntry

	if existing != nil {
		cfg = existing
		hadExisting = true
	} else {
		cfg = new(ConfigEntry)
	}

	if _, ok := d.Raw["anonymous_group_search"]; ok || !hadExisting {
		cfg.AnonymousGroupSearch = d.Get("anonymous_group_search").(bool)
	}

	if _, ok := d.Raw["url"]; ok || !hadExisting {
		cfg.Url = strings.ToLower(d.Get("url").(string))
	}

	if _, ok := d.Raw["userattr"]; ok || !hadExisting {
		cfg.UserAttr = strings.ToLower(d.Get("userattr").(string))
	}

	if _, ok := d.Raw["userdn"]; ok || !hadExisting {
		cfg.UserDN = d.Get("userdn").(string)
	}

	if _, ok := d.Raw["groupdn"]; ok || !hadExisting {
		cfg.GroupDN = d.Get("groupdn").(string)
	}

	if _, ok := d.Raw["groupfilter"]; ok || !hadExisting {
		groupfilter := d.Get("groupfilter").(string)
		if groupfilter != "" {
			// Validate the template before proceeding
			_, err := template.New("queryTemplate").Parse(groupfilter)
			if err != nil {
				return nil, errwrap.Wrapf("invalid groupfilter: {{err}}", err)
			}
		}

		cfg.GroupFilter = groupfilter
	}

	if _, ok := d.Raw["groupattr"]; ok || !hadExisting {
		cfg.GroupAttr = d.Get("groupattr").(string)
	}

	if _, ok := d.Raw["upndomain"]; ok || !hadExisting {
		cfg.UPNDomain = d.Get("upndomain").(string)
	}

	if _, ok := d.Raw["certificate"]; ok || !hadExisting {
		certificate := d.Get("certificate").(string)
		if certificate != "" {
			if err := validateCertificate([]byte(certificate)); err != nil {
				return nil, errwrap.Wrapf("failed to parse server tls cert: {{err}}", err)
			}
		}
		cfg.Certificate = certificate
	}

	if _, ok := d.Raw["client_tls_cert"]; ok || !hadExisting {
		clientTLSCert := d.Get("client_tls_cert").(string)
		cfg.ClientTLSCert = clientTLSCert
	}

	if _, ok := d.Raw["client_tls_key"]; ok || !hadExisting {
		clientTLSKey := d.Get("client_tls_key").(string)
		cfg.ClientTLSKey = clientTLSKey
	}

	if cfg.ClientTLSCert != "" && cfg.ClientTLSKey != "" {
		if _, err := tls.X509KeyPair([]byte(cfg.ClientTLSCert), []byte(cfg.ClientTLSKey)); err != nil {
			return nil, errwrap.Wrapf("failed to parse client X509 key pair: {{err}}", err)
		}
	} else if cfg.ClientTLSCert != "" || cfg.ClientTLSKey != "" {
		return nil, fmt.Errorf("both client_tls_cert and client_tls_key must be set")
	}

	if _, ok := d.Raw["insecure_tls"]; ok || !hadExisting {
		cfg.InsecureTLS = d.Get("insecure_tls").(bool)
	}

	if _, ok := d.Raw["tls_min_version"]; ok || !hadExisting {
		cfg.TLSMinVersion = d.Get("tls_min_version").(string)
		_, ok = tlsutil.TLSLookup[cfg.TLSMinVersion]
		if !ok {
			return nil, errors.New("invalid 'tls_min_version'")
		}
	}

	if _, ok := d.Raw["tls_max_version"]; ok || !hadExisting {
		cfg.TLSMaxVersion = d.Get("tls_max_version").(string)
		_, ok = tlsutil.TLSLookup[cfg.TLSMaxVersion]
		if !ok {
			return nil, fmt.Errorf("invalid 'tls_max_version'")
		}
	}
	if cfg.TLSMaxVersion < cfg.TLSMinVersion {
		return nil, fmt.Errorf("'tls_max_version' must be greater than or equal to 'tls_min_version'")
	}

	if _, ok := d.Raw["starttls"]; ok || !hadExisting {
		cfg.StartTLS = d.Get("starttls").(bool)
	}

	if _, ok := d.Raw["binddn"]; ok || !hadExisting {
		cfg.BindDN = d.Get("binddn").(string)
	}

	if _, ok := d.Raw["bindpass"]; ok || !hadExisting {
		cfg.BindPassword = d.Get("bindpass").(string)
	}

	if _, ok := d.Raw["deny_null_bind"]; ok || !hadExisting {
		cfg.DenyNullBind = d.Get("deny_null_bind").(bool)
	}

	if _, ok := d.Raw["discoverdn"]; ok || !hadExisting {
		cfg.DiscoverDN = d.Get("discoverdn").(bool)
	}

	if _, ok := d.Raw["case_sensitive_names"]; ok || !hadExisting {
		cfg.CaseSensitiveNames = new(bool)
		*cfg.CaseSensitiveNames = d.Get("case_sensitive_names").(bool)
	}

	usePre111GroupCNBehavior, ok := d.GetOk("use_pre111_group_cn_behavior")
	if ok {
		cfg.UsePre111GroupCNBehavior = new(bool)
		*cfg.UsePre111GroupCNBehavior = usePre111GroupCNBehavior.(bool)
	}

	if _, ok := d.Raw["use_token_groups"]; ok || !hadExisting {
		cfg.UseTokenGroups = d.Get("use_token_groups").(bool)
	}

	if _, ok := d.Raw["request_timeout"]; ok || !hadExisting {
		cfg.RequestTimeout = d.Get("request_timeout").(int)
	}

	return cfg, nil
}

type ConfigEntry struct {
	Url                      string `json:"url"`
	UserDN                   string `json:"userdn"`
	AnonymousGroupSearch     bool   `json:"anonymous_group_search"`
	GroupDN                  string `json:"groupdn"`
	GroupFilter              string `json:"groupfilter"`
	GroupAttr                string `json:"groupattr"`
	UPNDomain                string `json:"upndomain"`
	UserAttr                 string `json:"userattr"`
	Certificate              string `json:"certificate"`
	InsecureTLS              bool   `json:"insecure_tls"`
	StartTLS                 bool   `json:"starttls"`
	BindDN                   string `json:"binddn"`
	BindPassword             string `json:"bindpass"`
	DenyNullBind             bool   `json:"deny_null_bind"`
	DiscoverDN               bool   `json:"discoverdn"`
	TLSMinVersion            string `json:"tls_min_version"`
	TLSMaxVersion            string `json:"tls_max_version"`
	UseTokenGroups           bool   `json:"use_token_groups"`
	UsePre111GroupCNBehavior *bool  `json:"use_pre111_group_cn_behavior"`
	RequestTimeout           int    `json:"request_timeout"`

	// These json tags deviate from snake case because there was a past issue
	// where the tag was being ignored, causing it to be jsonified as "CaseSensitiveNames", etc.
	// To continue reading in users' previously stored values,
	// we chose to carry that forward.
	CaseSensitiveNames *bool  `json:"CaseSensitiveNames,omitempty"`
	ClientTLSCert      string `json:"ClientTLSCert"`
	ClientTLSKey       string `json:"ClientTLSKey"`
}

func (c *ConfigEntry) Map() map[string]interface{} {
	m := c.PasswordlessMap()
	m["bindpass"] = c.BindPassword
	return m
}

func (c *ConfigEntry) PasswordlessMap() map[string]interface{} {
	m := map[string]interface{}{
		"url":                    c.Url,
		"userdn":                 c.UserDN,
		"groupdn":                c.GroupDN,
		"groupfilter":            c.GroupFilter,
		"groupattr":              c.GroupAttr,
		"upndomain":              c.UPNDomain,
		"userattr":               c.UserAttr,
		"certificate":            c.Certificate,
		"insecure_tls":           c.InsecureTLS,
		"starttls":               c.StartTLS,
		"binddn":                 c.BindDN,
		"deny_null_bind":         c.DenyNullBind,
		"discoverdn":             c.DiscoverDN,
		"tls_min_version":        c.TLSMinVersion,
		"tls_max_version":        c.TLSMaxVersion,
		"use_token_groups":       c.UseTokenGroups,
		"anonymous_group_search": c.AnonymousGroupSearch,
		"request_timeout":        c.RequestTimeout,
	}
	if c.CaseSensitiveNames != nil {
		m["case_sensitive_names"] = *c.CaseSensitiveNames
	}
	if c.UsePre111GroupCNBehavior != nil {
		m["use_pre111_group_cn_behavior"] = *c.UsePre111GroupCNBehavior
	}
	return m
}

func validateCertificate(pemBlock []byte) error {
	block, _ := pem.Decode([]byte(pemBlock))
	if block == nil || block.Type != "CERTIFICATE" {
		return errors.New("failed to decode PEM block in the certificate")
	}
	_, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse certificate %s", err.Error())
	}
	return nil
}

func (c *ConfigEntry) Validate() error {
	if len(c.Url) == 0 {
		return errors.New("at least one url must be provided")
	}
	// Note: This logic is driven by the logic in GetUserBindDN.
	// If updating this, please also update the logic there.
	if !c.DiscoverDN && (c.BindDN == "" || c.BindPassword == "") && c.UPNDomain == "" && c.UserDN == "" {
		return errors.New("cannot derive UserBindDN")
	}
	tlsMinVersion, ok := tlsutil.TLSLookup[c.TLSMinVersion]
	if !ok {
		return errors.New("invalid 'tls_min_version' in config")
	}
	tlsMaxVersion, ok := tlsutil.TLSLookup[c.TLSMaxVersion]
	if !ok {
		return errors.New("invalid 'tls_max_version' in config")
	}
	if tlsMaxVersion < tlsMinVersion {
		return errors.New("'tls_max_version' must be greater than or equal to 'tls_min_version'")
	}
	if c.Certificate != "" {
		if err := validateCertificate([]byte(c.Certificate)); err != nil {
			return errwrap.Wrapf("failed to parse server tls cert: {{err}}", err)
		}
	}
	if c.ClientTLSCert != "" && c.ClientTLSKey != "" {
		if _, err := tls.X509KeyPair([]byte(c.ClientTLSCert), []byte(c.ClientTLSKey)); err != nil {
			return errwrap.Wrapf("failed to parse client X509 key pair: {{err}}", err)
		}
	}
	return nil
}
