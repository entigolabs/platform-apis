import React, { ReactNode } from 'react';
import Link from '@docusaurus/Link';
import useDocusaurusContext from '@docusaurus/useDocusaurusContext';

interface StrapiLinkProps {
    href: string;
    children: ReactNode;
    className?: string;
    target?: string;
    [key: string]: any;
}

export const StrapiLink = ({
    href,
    children,
    className = '',
    target,
    ...props
}: StrapiLinkProps) => {
    const { siteConfig } = useDocusaurusContext();
    const marketingUrl = siteConfig.customFields?.marketingUrl as string;

    if (!href) return <>{ children }</>;

    let formattedTarget = undefined;
    if (target) {
        formattedTarget = target.startsWith('_') ? target : `_${ target }`;
    }

    // Handle External vs Internal URLs
    const isExternal = href.startsWith('http');

    const formattedHref = href.startsWith('/') ? `${ marketingUrl }${ href }` : href;

    return (
        <Link
            to={ formattedHref }
            className={ className }
            target={ formattedTarget }
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

export const StrapiImage = ({
    src,
    alt,
    className = '',
    width,
    height
}: StrapiImageProps) => {
    const { siteConfig } = useDocusaurusContext();
    const strapiBaseUrl = siteConfig.customFields?.strapiUrl as string;

    if (!src) return null;

    const fullSrc = src.startsWith('http') ? src : `${ strapiBaseUrl }${ src }`;

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