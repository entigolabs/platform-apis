import React, { useState, useEffect, JSX } from 'react';
import { useLocation } from '@docusaurus/router';
import { usePluginData } from '@docusaurus/useGlobalData';
import { ReactSVG } from 'react-svg';
import classNames from 'classnames';
import { StrapiLink } from '@site/src/components/StrapiWrappers';
import styles from './styles.module.scss';

// Theme imports
import { useColorMode, useThemeConfig } from '@docusaurus/theme-common';
import ColorModeToggle from '@theme/ColorModeToggle';

import CheckmarkIcon from './checkmark.svg';
import BurgerIcon from './burger.svg';
import CrossIcon from './burger-cross.svg';

type NavbarLink = {
    __typename: string;
    text: string;
    href: string;
    target?: string;
    navbar_links?: NavbarLink[];
};

export default function Navbar(): JSX.Element | null {
    const pluginData = usePluginData('docusaurus-plugin-strapi-data') as any;
    if (!pluginData || !pluginData.headerNavbarContent) return null;

    const { headerNavbarContent } = pluginData;
    const location = useLocation();
    const currentRoute = location.pathname;

    const { colorMode, setColorMode } = useColorMode();
    const { colorMode: { respectPrefersColorScheme = true } = {} } = useThemeConfig();

    const [hamburgerOpen, setHamburgerOpen] = useState(false);
    const [serviceIndex, setServiceIndex] = useState<number | null>(null);

    useEffect(() => {
        setHamburgerOpen(false);
    }, [location]);

    const getLinkClass = (href: string) => {
        const isActive = currentRoute === href || (href !== '/' && currentRoute.startsWith(href));
        if (isActive) return 'text-entigo-blue';
        return 'nav-link-dynamic';
    };

    return (
        <div className="background-dark">

            <nav className={ classNames('navbar custom-navbar container') }>
                <div className="row">
                    <div className="col-12">
                        <div className="d-flex flex-wrap justify-content-space-between">

                            {/* LOGO */ }
                            <StrapiLink href="/" className="d-flex flex-align-items-center cursor-pointer my-28 my-lg-22 logo-link">
                                { headerNavbarContent.logo?.url && (
                                    <ReactSVG
                                        src={ headerNavbarContent.logo.url }
                                        beforeInjection={ (svg) => {
                                            svg.classList.add('d-block');
                                            svg.setAttribute("height", "40px");
                                            svg.setAttribute("width", "auto");
                                            svg.style.height = "40px";
                                            svg.style.width = "auto";
                                        } }
                                    />
                                ) }
                            </StrapiLink>

                            {/* DESKTOP MENU */ }
                            <div className="d-none d-lg-flex gap-32 align-items-center">
                                { headerNavbarContent.links.map((link: NavbarLink, index: number) => (
                                    <div key={ index + link.text }>
                                        {/* Single Link */ }
                                        { link.__typename === 'ComponentNavbarSingleLink' && (
                                            <StrapiLink target={ link.target } href={ link.href } className="text-decoration-none">
                                                <p className={ classNames('text-size-14 text-line-20 text-weight-600 cursor-pointer pb-32 pt-32 m-0', getLinkClass(link.href)) }>
                                                    { link.text }
                                                </p>
                                            </StrapiLink>
                                        ) }

                                        {/* Dropdown Link */ }
                                        { link.__typename === 'ComponentNavbarMultipleLinks' && (
                                            <div
                                                className="position-relative"
                                                onMouseEnter={ () => setServiceIndex(index) }
                                                onMouseLeave={ () => setServiceIndex(null) }
                                            >
                                                <div className="d-flex flex-align-items-center cursor-pointer">
                                                    <p className={ classNames('text-size-14 text-line-20 text-weight-600 pb-32 pt-32 mr-8 m-0 nav-link-dynamic') }>
                                                        { link.text }
                                                    </p>
                                                    <CheckmarkIcon className="checkmark-icon nav-icon-dynamic" />
                                                </div>

                                                <div className={ classNames(
                                                    styles.dropdown,
                                                    serviceIndex === index && styles.dropdownActive,
                                                    'absolute pl-16 pr-16 custom-navbar'
                                                ) }>
                                                    { link.navbar_links?.map((service: any, subIndex: number) => (
                                                        <StrapiLink key={ subIndex + service.text } target={ service.target } href={ service.href } className="d-block text-decoration-none">
                                                            <p className={ classNames('text-size-14 text-line-20 text-weight-600 mb-20 mt-0 cursor-pointer', getLinkClass(service.href)) }>
                                                                { service.text }
                                                            </p>
                                                        </StrapiLink>
                                                    )) }
                                                </div>
                                            </div>
                                        ) }
                                    </div>
                                )) }

                                <div className="d-flex flex-column justify-content-center flex-align-items-center">
                                    <StrapiLink href={ headerNavbarContent.cta_href } target={ headerNavbarContent.target } className="text-decoration-none">
                                        <button className={ classNames('btn-custom text-dark', styles.ctaButton) }>
                                            { headerNavbarContent.cta_text }
                                        </button>
                                    </StrapiLink>
                                </div>

                                {/* DESKTOP TOGGLE - Wrapped in nav-icon-dynamic so it picks up the color */ }
                                <div className="d-flex align-items-center nav-icon-dynamic">
                                    <ColorModeToggle
                                        value={ colorMode }
                                        onChange={ setColorMode }
                                        respectPrefersColorScheme={ respectPrefersColorScheme }
                                    />
                                </div>
                            </div>

                            {/* MOBILE HAMBURGER TOGGLE */ }
                            <div
                                className="d-lg-none d-flex flex-align-items-center justify-content-center cursor-pointer burger-icon"
                                onClick={ () => setHamburgerOpen(!hamburgerOpen) }
                                style={ { height: '40px', width: '40px' } }
                            >
                                { hamburgerOpen ? (
                                    <CrossIcon className="nav-icon-dynamic pr-8" />
                                ) : (
                                    <BurgerIcon className="nav-icon-dynamic" />
                                ) }
                            </div>
                        </div>
                    </div>
                </div>

                {/* MOBILE MENU OVERLAY */ }
                { hamburgerOpen && (
                    <div className={ classNames("mobile-menu pt-30 px-30 d-lg-none custom-navbar") }>
                        { headerNavbarContent.links.map((link: NavbarLink, index: number) => (
                            <div key={ index + link.text }>
                                { link.__typename === 'ComponentNavbarSingleLink' && (
                                    <StrapiLink href={ link.href } onClick={ () => setHamburgerOpen(false) }>
                                        <p className={ classNames('text-size-18 text-line-20 cursor-pointer pb-32 pt-32 m-0', getLinkClass(link.href)) }>
                                            { link.text }
                                        </p>
                                    </StrapiLink>
                                ) }

                                { link.__typename === 'ComponentNavbarMultipleLinks' && (
                                    <div className="d-flex flex-column position-relative">
                                        <div
                                            className="d-flex flex-align-items-center justify-content-space-between cursor-pointer"
                                            onClick={ () => setServiceIndex(serviceIndex === index ? null : index) }
                                        >
                                            <p className="text-size-18 text-line-20 pb-32 pt-32 mr-8 m-0 nav-link-dynamic">
                                                { link.text }
                                            </p>
                                            <CheckmarkIcon className="checkmark-icon nav-icon-dynamic" />
                                        </div>

                                        { serviceIndex === index && (
                                            <div className="custom-navbar">
                                                { link.navbar_links?.map((service: any, subIndex: number) => (
                                                    <StrapiLink key={ subIndex } href={ service.href } onClick={ () => setHamburgerOpen(false) }>
                                                        <p className={ classNames('text-size-16 text-line-20 mb-20 mt-0 cursor-pointer', getLinkClass(service.href)) }>
                                                            { service.text }
                                                        </p>
                                                    </StrapiLink>
                                                )) }
                                            </div>
                                        ) }
                                    </div>
                                ) }
                            </div>
                        )) }

                        <div className="d-flex my-28 my-lg-22 align-items-center justify-content-space-between">
                            <StrapiLink href={ headerNavbarContent.cta_href } target={ headerNavbarContent.target }
                                onClick={ () => setHamburgerOpen(false) }
                                className={ classNames('btn-custom text-decoration-none', styles.ctaButton, 'text-white') }>
                                { headerNavbarContent.cta_text }
                            </StrapiLink>

                            {/* MOBILE TOGGLE - Wrapped in nav-icon-dynamic */ }
                            <div className="nav-icon-dynamic">
                                <ColorModeToggle
                                    value={ colorMode }
                                    onChange={ setColorMode }
                                    respectPrefersColorScheme={ respectPrefersColorScheme }
                                />
                            </div>
                        </div>
                    </div>
                ) }
            </nav>
        </div>
    );
}