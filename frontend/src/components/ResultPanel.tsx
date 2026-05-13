import { EvidenceItem, RecognizeSearchResponse, VisualEvidence } from '../types';
import { SearchResultList } from './SearchResultList';

interface ResultPanelProps {
  data: RecognizeSearchResponse;
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

export function ResultPanel({ data }: ResultPanelProps) {
  const evidence = data.recognizedObject.visualEvidence;

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

      {hasEvidence(evidence) && (
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

      <div className="p-4 bg-white rounded-lg border border-neutral-200 mb-6">
        <h3 className="text-sm font-semibold text-neutral-900 mb-2">要約</h3>
        <p className="text-sm text-neutral-700">{data.summary.text}</p>
      </div>

      <SearchResultList results={data.search.results} />
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
