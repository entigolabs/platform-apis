import React from 'react';
import Admonition from '@theme/Admonition';

export default function TierNotice({ tiers }) {
    if (!tiers || tiers.length === 0) return null;
    
    return (
        <Admonition type="note" title="Tiers">
            This feature is available for the following tiers: <strong>{tiers.join(', ')}</strong>.
        </Admonition>
    );
}
