import { useParams } from 'react-router-dom';
import { Notice } from '@wordpress/components';

export default function IssueDetail() {
  const { id } = useParams();
  return (
    <div>
      <h1>Issue {id}</h1>
      <Notice status="info" isDismissible={false}>
        Issue detail view lands in Phase 2 (review/approval). This is a placeholder.
      </Notice>
    </div>
  );
}
