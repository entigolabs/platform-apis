import React, { ReactNode } from 'react';
import Link from '@docusaurus/Link';
import useDocusaurusContext from '@docusaurus/useDocusaurusContext';

// 1. Define Props
interface StrapiLinkProps {
    href: string;
    children: ReactNode;
    className?: string;
    target?: string;
    [key: string]: any;
}

// 2. StrapiLink Component
export const StrapiLink = ({
    href,
    children,
    className = '',
    target,
    ...props
}: StrapiLinkProps) => {
    // Get variables from Docusaurus Config
    const { siteConfig } = useDocusaurusContext();
    const marketingUrl = siteConfig.customFields?.marketingUrl as string;

    // Safety: If href is missing, just render children
    if (!href) return <>{ children }</>;

    // 1. Handle Target (add underscore if missing)
    let formattedTarget = undefined;
    if (target) {
        formattedTarget = target.startsWith('_') ? target : `_${ target }`;
    }

    // 2. Handle External vs Internal URLs
    // logic: If it starts with http, it's external.
    // If it's relative (e.g. /about), prepend the MARKETING_URL.
    const isExternal = href.startsWith('http');

    // Fallback: If marketingUrl is missing, just use the raw href to prevent "undefined/about"
    const formattedHref = href.startsWith('/') ? `${ marketingUrl }${ href }` : href;

    return (
        <Link
            to={ formattedHref }
            className={ className }
            target={ formattedTarget }
            // Add rel="noopener noreferrer" automatically for external links
            { ...(isExternal || formattedTarget === '_blank' ? { rel: 'noopener noreferrer' } : {}) }
            { ...props }
        >
            { children }
        </Link>
    );
};

interface StrapiImageProps {
    src: string;
    alt: string;
    className?: string;
    width?: number | string;
    height?: number | string;
}

// 3. StrapiImage Component
export const StrapiImage = ({
    src,
    alt,
    className = '',
    width,
    height
}: StrapiImageProps) => {
    const { siteConfig } = useDocusaurusContext();
    const strapiBase = siteConfig.customFields?.strapiUrl as string;

    if (!src) return null;

    // Ensure full URL if Strapi returns relative paths
    const fullSrc = src.startsWith('http') ? src : `${ strapiBase }${ src }`;

    return (
        <img
            src={ fullSrc }
            alt={ alt }
            className={ className }
            width={ width }
            height={ height }
            loading="lazy"
        />
    );
};