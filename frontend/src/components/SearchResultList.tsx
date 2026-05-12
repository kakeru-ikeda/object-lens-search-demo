import { NormalizedSearchResult } from '../types';

interface SearchResultListProps {
  results: NormalizedSearchResult[];
}

export function SearchResultList({ results }: SearchResultListProps) {
  return (
    <div className="flex flex-col gap-4 mt-4">
      <h3 className="text-lg font-semibold text-neutral-900">検索結果</h3>
      {results.map((result) => (
        <a 
          key={result.id} 
          href={result.url}
          target="_blank"
          rel="noopener noreferrer"
          className="flex flex-col gap-1 p-3 bg-white border border-neutral-200 rounded-lg shadow-sm hover:border-blue-500 transition-colors"
        >
          <span className="text-xs text-neutral-500">{result.source}</span>
          <h4 className="text-base font-medium text-blue-600 line-clamp-2">{result.title}</h4>
          <p className="text-sm text-neutral-600 line-clamp-2">{result.snippet}</p>
        </a>
      ))}
    </div>
  );
}
