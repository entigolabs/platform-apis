import React, { JSX } from 'react';
import { usePluginData } from '@docusaurus/useGlobalData';
import { useLocation } from '@docusaurus/router';
import classNames from 'classnames';
import { StrapiLink, StrapiImage } from '@site/src/components/StrapiWrappers';
import { ReactSVG } from 'react-svg';
import { useColorMode } from '@docusaurus/theme-common';

export default function Footer(): JSX.Element | null {
    const pluginData = usePluginData('docusaurus-plugin-strapi-data') as any;
    if (!pluginData || !pluginData.footerNavbarContent) return null;

    const { footerNavbarContent, footerData } = pluginData;
    const location = useLocation();
    const currentRoute = location.pathname;

    const { colorMode } = useColorMode();
    const isDark = colorMode === 'dark';

    const getLinkColorClass = (href: string) => {
        const isActive = currentRoute === href;
        if (isActive) return 'text-primary-blue';
        return isDark
            ? 'text-light-blue hover-text-primary-blue'
            : 'text-hover-blue hover-text-primary-blue';
    };

    const getTextColorClass = () => isDark ? 'text-light-grey' : 'text-dark';

    return (
        <footer className={ classNames('footer-custom', isDark ? 'background-dark' : 'background-light-grey') }>
            <div className="container">

                {/* 1. TOP LINKS SECTION */ }
                <div className={ classNames("row pt-36 pb-36", isDark ? 'dark-mode-bottom-border' : 'light-mode-bottom-border') }>
                    <div className="d-flex flex-wrap justify-content-space-between w-100 px-12">
                        { footerNavbarContent.links?.map((link: any, index: number) => (
                            <div
                                className="d-flex flex-column mb-24"
                                key={ index }
                                style={ { minWidth: '150px' } }
                            >
                                <p className={ classNames("text-size-14 text-line-20 text-weight-700 mt-16 mb-12", getTextColorClass()) }>
                                    { link.text }
                                </p>

                                { link.navbar_links?.map((sublink: any, subIndex: number) => (
                                    <StrapiLink
                                        href={ sublink.href }
                                        target={ sublink.target }
                                        key={ subIndex }
                                        className="d-block text-decoration-none"
                                    >
                                        <p className={ classNames('text-size-14 text-line-20 text-weight-500 mt-16 mb-8 cursor-pointer', getLinkColorClass(sublink.href)) }>
                                            { sublink.text }
                                        </p>
                                    </StrapiLink>
                                )) || (
                                        <StrapiLink
                                            href={ link.href }
                                            target={ link.target }
                                            className="d-block text-decoration-none"
                                        >
                                            <p className={ classNames('text-size-14 text-line-20 text-weight-500 mt-16 mb-8 cursor-pointer', getLinkColorClass(link.href)) }>
                                                { link.text }
                                            </p>
                                        </StrapiLink>
                                    ) }
                            </div>
                        )) }
                    </div>
                </div>

                {/* 2. PARTNERS AND MEDIA */ }
                { footerData && (
                    <div className="row pt-lg-54 pt-36 pb-lg-54">
                        <div className="col-12 d-lg-none mb-36">
                            <div className="d-flex gap-24 flex-align-items-center justify-content-center">
                                { footerData.footer_media?.map((media: any, i: number) => (
                                    <StrapiLink href={ media.href } key={ i } target="_blank">
                                        { media.svg_icon?.url && (
                                            <ReactSVG
                                                src={ media.svg_icon.url }
                                                beforeInjection={ svg => {
                                                    svg.classList.add('d-block');
                                                    svg.setAttribute("height", "43px");
                                                    svg.setAttribute("width", "auto");
                                                } }
                                            />
                                        ) }
                                    </StrapiLink>
                                )) }
                            </div>
                        </div>

                        {/* Mobile Partner Logos */ }
                        <div className="col-12 d-lg-none">
                            <div className="d-flex flex-wrap gap-24 flex-align-items-center justify-content-center" style={ { rowGap: '16px' } }>
                                { footerData.partners?.map((partner: any, i: number) => (
                                    <div key={ i } className="d-flex flex-align-items-center" style={ { padding: '8px' } }>
                                        <StrapiLink href={ partner.href } target="_blank">
                                            <StrapiImage src={ partner.logo?.url } alt={ partner.name } width={ 43 } height={ 43 } className="d-block" />
                                        </StrapiLink>
                                    </div>
                                )) }
                            </div>
                        </div>

                        {/* Desktop Partner Logos and Media Icons */ }
                        <div className="d-none d-lg-flex justify-content-space-between w-100 px-12">
                            <div className="d-flex flex-wrap justify-content-center justify-content-lg-start mb-36 mb-lg-0">
                                { footerData.partners?.map((partner: any, i: number) => (
                                    <div key={ i } className="d-flex flex-align-items-center" style={ { padding: '8px' } }>
                                        <StrapiLink href={ partner.href } target="_blank">
                                            <StrapiImage src={ partner.logo?.url } alt={ partner.name } width={ 43 } height={ 43 } className="d-block" />
                                        </StrapiLink>
                                    </div>
                                )) }
                            </div>

                            {/* Desktop Media Icons */ }
                            <div className="d-flex col-3 gap-24 flex-align-items-center justify-content-flex-end">
                                { footerData.footer_media?.map((media: any, i: number) => (
                                    <StrapiLink href={ media.href } key={ i } target="_blank">
                                        { media.svg_icon?.url && (
                                            <ReactSVG
                                                src={ media.svg_icon.url }
                                                beforeInjection={ svg => {
                                                    svg.classList.add('d-block');
                                                    svg.setAttribute("height", "43px");
                                                    svg.setAttribute("width", "auto");
                                                } }
                                            />
                                        ) }
                                    </StrapiLink>
                                )) }
                            </div>
                        </div>

                        {/* 3. FOOTER BOTTOM SECTION */ }
                        <div className="d-flex flex-wrap w-100">
                            { footerData.contact_info && (
                                <div className="col-12 col-lg-6">
                                    <p className={ classNames("text-lg-right text-center text-size-12 text-line-20 text-weight-400 ml-54 mr-54 ml-lg-0 mr-lg-0 mb-8 my-lg-16", getTextColorClass()) }>
                                        { footerData.contact_info }
                                    </p>
                                </div>
                            ) }

                            <div className="col-12 col-lg-6 d-flex gap-10 gap-lg-0 order-lg-first justify-content-center justify-content-lg-flex-start flex-wrap">
                                { footerData.copyright && (
                                    <p className={ classNames("text-lg-left text-center text-size-14 text-weight-400 text-line-20 mt-0 mt-lg-16 mb-38 mb-lg-16", getTextColorClass()) }>
                                        { footerData.copyright }
                                    </p>
                                ) }

                                <div className="d-flex mb-4 mb-lg-16 mt-0 mt-lg-16 ml-lg-30">
                                    <StrapiLink href={ footerData.privacy_policy_href || '/privacy-policy' }>
                                        <p className={ classNames("text-lg-left text-center text-size-12 text-line-20 cursor-pointer text-weight-400 m-0", getLinkColorClass('/privacy-policy')) }>
                                            { footerData.privacy_policy_cta || 'Privacy Policy' }
                                        </p>
                                    </StrapiLink>
                                </div>

                                <div className="d-flex mb-4 mb-lg-16 mt-0 mt-lg-16 ml-lg-30">
                                    <StrapiLink href="/cookie-preferences">
                                        <p className={ classNames("text-lg-left text-center text-size-12 text-line-20 cursor-pointer text-weight-400 m-0", getLinkColorClass('/cookie-preferences')) }>
                                            { footerData.cookies_cta || 'Manage Cookie Preferences' }
                                        </p>
                                    </StrapiLink>
                                </div>
                            </div>
                        </div>
                    </div>
                ) }
            </div>
        </footer>
    );
}