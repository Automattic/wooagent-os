import { Stack } from '@wordpress/ui';
import { Button } from '@wordpress/components';

interface Props {
  onOpenKanban(): void;
}

export default function Step5Done({ onOpenKanban }: Props) {
  return (
    <Stack direction="row" justify="center" align="center">
      <Button
        variant="primary"
        __next40pxDefaultSize
        onClick={onOpenKanban}
      >
        Get started
      </Button>
    </Stack>
  );
}
