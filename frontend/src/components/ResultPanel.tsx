import { EvidenceItem, RecognizeSearchResponse, VisualEvidence } from '../types';
import { SearchResultList } from './SearchResultList';
import { Loader2, X, AlertTriangle } from 'lucide-react';
import { PartialData } from '../hooks/useRecognizeSearch';

interface ResultPanelProps {
  data: RecognizeSearchResponse | null;
  partialData?: PartialData | null;
  loading?: boolean;
  error?: Error | null;
  onClose: () => void;
}

const statusLabel: Record<string, string> = {
  measured: 'Cloud Vision 証拠を取得済み',
  cloud_vision_no_evidence: 'Cloud Vision は実行済み（証拠なし）',
  cloud_vision_error: 'Cloud Vision 実行エラー（検索は継続）',
  cloud_vision_disabled: 'Cloud Vision は無効',
  multi_crop_received_cloud_vision_disabled: 'multi-crop 受信済み / Cloud Vision は無効',
  multi_crop_received_not_measured: 'multi-crop 受信済み / 未測定',
  not_measured: '未測定',
};

function itemText(items?: EvidenceItem[]) {
  return items?.map((item) => item.text).filter(Boolean).join(', ') ?? '';
}

function hasEvidence(evidence?: VisualEvidence) {
  return Boolean(
    evidence &&
      ((evidence.ocr?.length ?? 0) > 0 ||
        (evidence.logos?.length ?? 0) > 0 ||
        (evidence.webEntities?.length ?? 0) > 0 ||
        (evidence.bestGuessLabels?.length ?? 0) > 0 ||
        (evidence.labels?.length ?? 0) > 0 ||
        (evidence.matchingImageUrls?.length ?? 0) > 0),
  );
}

function headlineFor(data: RecognizeSearchResponse) {
  const object = data.recognizedObject;
  return object.finalObjectName || object.displayName || object.objectName;
}

export function ResultPanel({ data, partialData, loading, error, onClose }: ResultPanelProps) {
  const isFinal = !!data;
  const showHypothesis = !isFinal && partialData?.hypothesis;
  const showQuery = !isFinal && partialData?.query;
  const showResults = !isFinal && partialData?.searchResults;
  const evidence = data?.recognizedObject.visualEvidence;
  const category = data?.recognizedObject.category;

  return (
    <div className="flex flex-col w-full max-w-md mx-auto bg-neutral-50 rounded-t-2xl p-5 sm:p-6 overflow-y-auto max-h-[min(78dvh,calc(100dvh-env(safe-area-inset-top)-env(safe-area-inset-bottom)-1rem))] shadow-xl">
      <div className="flex justify-between items-start mb-4">
        <div>
          {isFinal ? (
            <>
              <h2 className="text-xl font-bold text-neutral-900">{headlineFor(data)}</h2>
              <div className="mt-2 flex flex-wrap gap-2">
                <span className="text-xs font-medium px-2 py-1 bg-blue-100 text-blue-700 rounded-full inline-block">
                  {data.recognizedObject.confidence} confidence
                </span>
                {category && (
                  <span className="text-xs font-medium px-2 py-1 bg-neutral-200 text-neutral-700 rounded-full inline-block">
                    {category}
                  </span>
                )}
              </div>
            </>
          ) : (
            <div className="flex items-center gap-2">
              <h2 className="text-xl font-bold text-neutral-900">
                {error ? 'エラーが発生しました' : '検索中...'}
              </h2>
              {loading && <Loader2 className="w-5 h-5 animate-spin text-blue-500" />}
            </div>
          )}
        </div>
        <button
          type="button"
          onClick={onClose}
          className="ml-4 inline-flex h-9 w-9 shrink-0 items-center justify-center rounded-full bg-neutral-900 text-white shadow-sm transition-colors hover:bg-neutral-700"
          aria-label="検索結果を閉じる"
        >
          <X className="h-4 w-4" />
        </button>
      </div>

      {error && (
        <div className="p-4 bg-red-50 text-red-700 rounded-lg border border-red-200 mb-6 flex items-start gap-3">
          <AlertTriangle className="w-5 h-5 shrink-0 mt-0.5" />
          <p className="text-sm font-medium">{error.message}</p>
        </div>
      )}

      {isFinal ? (
        <p className="text-neutral-700 mb-6">{data.recognizedObject.description}</p>
      ) : showHypothesis ? (
        <div className="p-4 bg-amber-50 rounded-lg border border-amber-200 mb-6">
          <div className="flex items-center gap-2 mb-2">
            <span className="relative flex h-2 w-2">
              {loading && <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-amber-400 opacity-75"></span>}
              <span className="relative inline-flex rounded-full h-2 w-2 bg-amber-500"></span>
            </span>
            <h3 className="text-sm font-semibold text-amber-900">LLMの仮説 (未検証)</h3>
          </div>
          <p className="text-sm text-amber-800 italic">{partialData.hypothesis}</p>
          {showQuery && (
            <p className="text-xs text-amber-700 mt-2 font-mono bg-amber-100 p-2 rounded">
              検索クエリ: {partialData.query}
            </p>
          )}
        </div>
      ) : null}

      {isFinal && (data.inputSummary || data.evidenceFusion) && (
        <div className="p-4 bg-white rounded-lg border border-neutral-200 mb-4">
          <h3 className="text-sm font-semibold text-neutral-900 mb-2">複数画像の統合</h3>
          {data.inputSummary && (
            <p className="text-xs text-neutral-700">
              {data.inputSummary.imageCount}枚の画像を統合 · primary: {data.inputSummary.primaryImageId}
            </p>
          )}
          {data.evidenceFusion && (
            <div className="mt-2 space-y-1 text-xs text-neutral-700">
              <p>coverage: <span className="font-medium">{data.evidenceFusion.coverage}</span></p>
              <p>agreement: <span className="font-medium">{data.evidenceFusion.agreement}</span></p>
              {data.evidenceFusion.signals?.length ? <p>signals: {data.evidenceFusion.signals.join(", ")}</p> : null}
            </div>
          )}
        </div>
      )}

      {isFinal && (
        <div className="p-4 bg-white rounded-lg border border-neutral-200 mb-4">
          <h3 className="text-sm font-semibold text-neutral-900 mb-2">画像入力シグナル</h3>
          <div className="grid grid-cols-3 gap-2 text-xs text-neutral-700">
            <div>
              <span className="block text-neutral-500">crop</span>
              <span className="font-medium">{data.queryQuality.cropConfidence}</span>
            </div>
            <div>
              <span className="block text-neutral-500">blur</span>
              <span className="font-medium">{data.queryQuality.blur}</span>
            </div>
            <div>
              <span className="block text-neutral-500">text</span>
              <span className="font-medium">{data.queryQuality.textVisibility}</span>
            </div>
          </div>
          <p className="mt-3 text-xs text-neutral-500">
            {statusLabel[data.queryQuality.status] ?? data.queryQuality.status}
            {data.queryQuality.evidenceTypes?.length ? ` / 証拠: ${data.queryQuality.evidenceTypes.join(', ')}` : ''}
          </p>
          {data.ambiguity.isAmbiguous && (
            <p className="mt-3 text-xs text-amber-700 bg-amber-50 rounded-md px-3 py-2">
              判定が曖昧です: {data.ambiguity.reason}
            </p>
          )}
        </div>
      )}

      {isFinal && hasEvidence(evidence) && (
        <div className="p-4 bg-white rounded-lg border border-neutral-200 mb-4">
          <h3 className="text-sm font-semibold text-neutral-900 mb-2">Cloud Vision 証拠</h3>
          <div className="space-y-2 text-xs text-neutral-700">
            {itemText(evidence?.ocr) && <EvidenceRow label="OCR" value={itemText(evidence?.ocr)} />}
            {itemText(evidence?.logos) && <EvidenceRow label="ロゴ" value={itemText(evidence?.logos)} />}
            {itemText(evidence?.webEntities) && <EvidenceRow label="Web" value={itemText(evidence?.webEntities)} />}
            {evidence?.bestGuessLabels?.length ? <EvidenceRow label="推定" value={evidence.bestGuessLabels.join(', ')} /> : null}
            {itemText(evidence?.labels) && <EvidenceRow label="ラベル" value={itemText(evidence?.labels)} />}
            {evidence?.matchingImageUrls?.length ? <EvidenceRow label="一致画像" value={`${evidence.matchingImageUrls.length}件`} /> : null}
          </div>
        </div>
      )}

      {isFinal && (
        <div className="p-4 bg-white rounded-lg border border-neutral-200 mb-6">
          <h3 className="text-sm font-semibold text-neutral-900 mb-2">要約</h3>
          <p className="text-sm text-neutral-700">{data.summary.text}</p>
        </div>
      )}

      {isFinal && data.search.results.length > 0 && <SearchResultList results={data.search.results} />}
      {!isFinal && showResults && partialData.searchResults && partialData.searchResults.length > 0 && (
        <div className="opacity-70 grayscale-[20%] transition-all">
          <SearchResultList results={partialData.searchResults} />
        </div>
      )}
    </div>
  );
}

function EvidenceRow({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <span className="text-neutral-500">{label}:</span>
      <span className="ml-2 font-medium">{value}</span>
    </div>
  );
}
