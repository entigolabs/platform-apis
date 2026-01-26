const path = require('path');

require('dotenv').config({ path: path.resolve(__dirname, '../../../.env') });

const NAVBAR_QUERY = `
    query Navbars($filters: NavbarFiltersInput, $locale: I18NLocaleCode) {
      navbars(filters: $filters, locale: $locale) {
        logo {
          url
        }
        links {
          __typename
          ... on ComponentNavbarSingleLink {
            href
            target
            text
          }
          ... on ComponentNavbarMultipleLinks {
            text
            navbar_links {
              href
              target
              text
            }
          }
        }
        cta_href
        cta_text
        target
      }
    }
`;

const FOOTER_DATA_QUERY = `
    query Footer($locale: I18NLocaleCode) {
      footer(locale: $locale) {
        footer_media {
        href
        name
        svg_icon {
            url
        }
        }
        partners {
        href
        name
        logo {
            url
        }
        }
        contact_info
        copyright
        privacy_policy_href
        privacy_policy_cta
        cookies_cta
      }
    }
`;

module.exports = function strapiDataPlugin(context, options) {
    return {
        name: 'docusaurus-plugin-strapi-data',

        async loadContent() {
            const STRAPI_URL = process.env.STRAPI_URL;
            const STRAPI_TOKEN = process.env.STRAPI_API_TOKEN;

            if (!STRAPI_URL || !STRAPI_TOKEN) {
                console.warn('⚠️ STRAPI_URL or STRAPI_API_TOKEN is missing in .env');
                return { headerNavbarContent: null, footerNavbarContent: null, footerData: null };
            }

            const fetchGraphQL = async (query, variables) => {
                try {
                    const response = await fetch(`${STRAPI_URL}/graphql`, {
                        method: 'POST',
                        headers: {
                            'Content-Type': 'application/json',
                            'Authorization': `Bearer ${STRAPI_TOKEN}`,
                        },
                        body: JSON.stringify({ query, variables }),
                    });

                    if (!response.ok) {
                        console.error(`❌ HTTP Error: ${response.status} ${response.statusText}`);
                        return null;
                    }

                    const json = await response.json();

                    if (json.errors) {
                        console.error('❌ Strapi GraphQL Errors:', JSON.stringify(json.errors, null, 2));
                        return null;
                    }

                    return json;
                } catch (error) {
                    console.error('❌ Network Error:', error.message);
                    return null;
                }
            };

            const [headerRes, footerNavRes, footerDataRes] = await Promise.all([
                fetchGraphQL(NAVBAR_QUERY, {
                    locale: 'en',
                    filters: { title: { eq: 'header-navbar' } }
                }),
                fetchGraphQL(NAVBAR_QUERY, {
                    locale: 'en',
                    filters: { title: { eq: 'footer-navbar' } }
                }),
                fetchGraphQL(FOOTER_DATA_QUERY, { locale: 'en' })
            ]);

            const headerNavbarContent = headerRes?.data?.navbars?.[0] || null;
            const footerNavbarContent = footerNavRes?.data?.navbars?.[0] || null;
            const footerData = footerDataRes?.data?.footer || null;

            headerNavbarContent ? console.log('✅ Header Navbar fetched') : console.log('⚠️ Header Navbar not found');
            footerNavbarContent ? console.log('✅ Footer Navbar fetched') : console.log('⚠️ Footer Navbar not found');
            footerData ? console.log('✅ Footer Data fetched') : console.log('⚠️ Footer Data not found');

            return {
                headerNavbarContent,
                footerNavbarContent,
                footerData
            };
        },

        async contentLoaded({ content, actions }) {
            const { setGlobalData } = actions;
            setGlobalData(content);
        },
    };
};