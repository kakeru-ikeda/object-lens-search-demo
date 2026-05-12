import { RecognizeSearchResponse } from '../types';
import { SearchResultList } from './SearchResultList';

interface ResultPanelProps {
  data: RecognizeSearchResponse;
}

export function ResultPanel({ data }: ResultPanelProps) {
  return (
    <div className="flex flex-col w-full max-w-md mx-auto bg-neutral-50 rounded-t-2xl p-6 overflow-y-auto max-h-[70vh] shadow-xl">
      <div className="flex justify-between items-start mb-4">
        <div>
          <h2 className="text-xl font-bold text-neutral-900">{data.recognizedObject.objectName}</h2>
          <span className="text-xs font-medium px-2 py-1 bg-blue-100 text-blue-700 rounded-full">
            {data.recognizedObject.confidence} confidence
          </span>
        </div>
      </div>
      
      <p className="text-neutral-700 mb-6">{data.recognizedObject.description}</p>
      
      <div className="p-4 bg-white rounded-lg border border-neutral-200 mb-6">
        <h3 className="text-sm font-semibold text-neutral-900 mb-2">要約</h3>
        <p className="text-sm text-neutral-700">{data.summary.text}</p>
      </div>

      <SearchResultList results={data.search.results} />
    </div>
  );
}
