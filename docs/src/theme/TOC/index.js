import React from 'react';
import TOC from '@theme-original/TOC';
import { useLocation } from '@docusaurus/router';

export default function TOCWrapper(props) {
  const location = useLocation();
  const params = new URLSearchParams(location.search);
  const tab = params.get('tab') || 'api-reference';

  // Filter headings based on tab
  const filteredToc = props.toc.filter(item => {
    if (tab === 'api-reference') {
      return !item.id.startsWith('example');
    }
    if (tab === 'examples') {
      return item.id.startsWith('example');
    }
    return false
  });

  return <TOC {...props} toc={filteredToc} />;
}