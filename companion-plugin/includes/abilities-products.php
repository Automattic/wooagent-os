<?php
/**
 * Product abilities for WooAgent.
 *
 * wooagent-products/list, wooagent-products/get, wooagent-products/update
 */

if ( ! defined( 'ABSPATH' ) ) {
	exit;
}

function wooagent_companion_register_product_abilities(): void {
	wp_register_ability(
		'wooagent-products/list',
		array(
			'label'               => __( 'List products', 'wooagent-companion' ),
			'description'         => __( 'List WooCommerce products with optional filters. Returns a lightweight summary per product.', 'wooagent-companion' ),
			'input_schema'        => array(
				'type'       => 'object',
				'properties' => array(
					'status'   => array(
						'type'        => 'string',
						'enum'        => array( 'any', 'publish', 'draft', 'pending', 'private' ),
						'default'     => 'publish',
						'description' => 'Product status filter.',
					),
					'per_page' => array(
						'type'        => 'integer',
						'minimum'     => 1,
						'maximum'     => 100,
						'default'     => 25,
						'description' => 'Items per page.',
					),
					'page'     => array(
						'type'    => 'integer',
						'minimum' => 1,
						'default' => 1,
					),
					'search'   => array(
						'type'        => 'string',
						'description' => 'Text search across product name and SKU.',
					),
				),
				'additionalProperties' => false,
			),
			'output_schema'       => array(
				'type'       => 'object',
				'properties' => array(
					'products' => array(
						'type'  => 'array',
						'items' => array(
							'type'       => 'object',
							'properties' => array(
								'id'                   => array( 'type' => 'integer' ),
								'name'                 => array( 'type' => 'string' ),
								'slug'                 => array( 'type' => 'string' ),
								'sku'                  => array( 'type' => 'string' ),
								'status'               => array( 'type' => 'string' ),
								'type'                 => array( 'type' => 'string' ),
								'regular_price'        => array( 'type' => 'string' ),
								'sale_price'           => array( 'type' => 'string' ),
								'on_sale'              => array( 'type' => 'boolean' ),
								'description_length'   => array( 'type' => 'integer' ),
								'short_description_length' => array( 'type' => 'integer' ),
								'featured'             => array( 'type' => 'boolean' ),
								'date_modified'        => array( 'type' => 'string' ),
							),
						),
					),
					'total'    => array( 'type' => 'integer' ),
				),
				'required'   => array( 'products', 'total' ),
			),
			'category'            => 'wooagent-products',
			'execute_callback'    => 'wooagent_products_list_execute',
			'permission_callback' => 'wooagent_products_read_permission',
			'meta'                => array(
				'show_in_rest' => true,
				'mcp'          => array( 'public' => true ),
				'annotations'  => array( 'readonly' => true, 'idempotent' => true ),
			),
		)
	);

	wp_register_ability(
		'wooagent-products/get',
		array(
			'label'               => __( 'Get product', 'wooagent-companion' ),
			'description'         => __( 'Read a single product including full description, short description, and SEO meta.', 'wooagent-companion' ),
			'input_schema'        => array(
				'type'       => 'object',
				'properties' => array(
					'id' => array(
						'type'        => 'integer',
						'minimum'     => 1,
						'description' => 'Product ID.',
					),
				),
				'required'   => array( 'id' ),
				'additionalProperties' => false,
			),
			'output_schema'       => array(
				'type'       => 'object',
				'properties' => array(
					'id'                => array( 'type' => 'integer' ),
					'name'              => array( 'type' => 'string' ),
					'slug'              => array( 'type' => 'string' ),
					'sku'               => array( 'type' => 'string' ),
					'status'            => array( 'type' => 'string' ),
					'type'              => array( 'type' => 'string' ),
					'regular_price'     => array( 'type' => 'string' ),
					'sale_price'        => array( 'type' => 'string' ),
					'description'       => array( 'type' => 'string' ),
					'short_description' => array( 'type' => 'string' ),
					'categories'        => array(
						'type'  => 'array',
						'items' => array(
							'type'       => 'object',
							'properties' => array(
								'id'   => array( 'type' => 'integer' ),
								'name' => array( 'type' => 'string' ),
							),
						),
					),
					'tags'              => array(
						'type'  => 'array',
						'items' => array( 'type' => 'string' ),
					),
					'meta_title'        => array( 'type' => 'string' ),
					'meta_description'  => array( 'type' => 'string' ),
					'featured'          => array( 'type' => 'boolean' ),
					'date_modified'     => array( 'type' => 'string' ),
					'image_url'         => array( 'type' => 'string' ),
					'image_alt'         => array( 'type' => 'string' ),
				),
				'required'   => array( 'id', 'name', 'status' ),
			),
			'category'            => 'wooagent-products',
			'execute_callback'    => 'wooagent_products_get_execute',
			'permission_callback' => 'wooagent_products_read_permission',
			'meta'                => array(
				'show_in_rest' => true,
				'mcp'          => array( 'public' => true ),
				'annotations'  => array( 'readonly' => true, 'idempotent' => true ),
			),
		)
	);

	wp_register_ability(
		'wooagent-products/update',
		array(
			'label'               => __( 'Update product', 'wooagent-companion' ),
			'description'         => __( 'Update any subset of editable product fields. Only the fields provided are modified.', 'wooagent-companion' ),
			'input_schema'        => array(
				'type'       => 'object',
				'properties' => array(
					'id'                => array( 'type' => 'integer', 'minimum' => 1 ),
					'name'              => array( 'type' => 'string' ),
					'description'       => array( 'type' => 'string' ),
					'short_description' => array( 'type' => 'string' ),
					'regular_price'     => array( 'type' => 'string', 'description' => 'Decimal string, e.g., "19.99".' ),
					'sale_price'        => array( 'type' => 'string' ),
					'status'            => array( 'type' => 'string', 'enum' => array( 'publish', 'draft', 'pending', 'private' ) ),
					'meta_title'        => array( 'type' => 'string' ),
					'meta_description'  => array( 'type' => 'string' ),
				),
				'required'   => array( 'id' ),
				'additionalProperties' => false,
			),
			'output_schema'       => array(
				'type'       => 'object',
				'properties' => array(
					'id'            => array( 'type' => 'integer' ),
					'updated_fields' => array( 'type' => 'array', 'items' => array( 'type' => 'string' ) ),
					'date_modified' => array( 'type' => 'string' ),
				),
				'required'   => array( 'id', 'updated_fields' ),
			),
			'category'            => 'wooagent-products',
			'execute_callback'    => 'wooagent_products_update_execute',
			'permission_callback' => 'wooagent_products_write_permission',
			'meta'                => array(
				'show_in_rest' => true,
				'mcp'          => array( 'public' => true ),
				'annotations'  => array( 'readonly' => false, 'destructive' => false, 'idempotent' => false ),
			),
		)
	);
}

function wooagent_products_read_permission(): bool {
	return current_user_can( 'read_private_products' ) || current_user_can( 'edit_products' );
}

function wooagent_products_write_permission(): bool {
	return current_user_can( 'edit_products' );
}

function wooagent_products_list_execute( array $args ) {
	if ( ! class_exists( 'WC_Product_Query' ) ) {
		return new WP_Error( 'wooagent_woocommerce_missing', __( 'WooCommerce is not active.', 'wooagent-companion' ) );
	}

	$query_args = array(
		'status'   => $args['status'] ?? 'publish',
		'limit'    => $args['per_page'] ?? 25,
		'page'     => $args['page'] ?? 1,
		'paginate' => true,
	);

	if ( 'any' === $query_args['status'] ) {
		$query_args['status'] = array( 'publish', 'draft', 'pending', 'private' );
	}

	if ( ! empty( $args['search'] ) ) {
		$query_args['s'] = $args['search'];
	}

	$result = wc_get_products( $query_args );

	$products = array();
	foreach ( $result->products as $product ) {
		$products[] = wooagent_product_to_summary( $product );
	}

	return array(
		'products' => $products,
		'total'    => (int) $result->total,
	);
}

function wooagent_products_get_execute( array $args ) {
	$product = wc_get_product( (int) $args['id'] );
	if ( ! $product ) {
		return new WP_Error( 'wooagent_product_not_found', __( 'Product not found.', 'wooagent-companion' ), array( 'status' => 404 ) );
	}

	$categories = array();
	foreach ( $product->get_category_ids() as $cat_id ) {
		$term = get_term( $cat_id, 'product_cat' );
		if ( $term && ! is_wp_error( $term ) ) {
			$categories[] = array( 'id' => (int) $term->term_id, 'name' => $term->name );
		}
	}

	$tags = array();
	foreach ( $product->get_tag_ids() as $tag_id ) {
		$term = get_term( $tag_id, 'product_tag' );
		if ( $term && ! is_wp_error( $term ) ) {
			$tags[] = $term->name;
		}
	}

	$image_id  = (int) $product->get_image_id();
	$image_url = '';
	$image_alt = '';
	if ( $image_id > 0 ) {
		$src = wp_get_attachment_image_src( $image_id, 'medium' );
		if ( is_array( $src ) && ! empty( $src[0] ) ) {
			$image_url = (string) $src[0];
		}
		$image_alt = (string) get_post_meta( $image_id, '_wp_attachment_image_alt', true );
	}

	return array(
		'id'                => $product->get_id(),
		'name'              => $product->get_name(),
		'slug'              => $product->get_slug(),
		'sku'               => $product->get_sku(),
		'status'            => $product->get_status(),
		'type'              => $product->get_type(),
		'regular_price'     => $product->get_regular_price(),
		'sale_price'        => $product->get_sale_price(),
		'description'       => $product->get_description(),
		'short_description' => $product->get_short_description(),
		'categories'        => $categories,
		'tags'              => $tags,
		'meta_title'        => (string) get_post_meta( $product->get_id(), '_yoast_wpseo_title', true ),
		'meta_description'  => (string) get_post_meta( $product->get_id(), '_yoast_wpseo_metadesc', true ),
		'featured'          => $product->is_featured(),
		'date_modified'     => $product->get_date_modified() ? $product->get_date_modified()->date( 'c' ) : '',
		'image_url'         => $image_url,
		'image_alt'         => $image_alt,
	);
}

function wooagent_products_update_execute( array $args ) {
	$product = wc_get_product( (int) $args['id'] );
	if ( ! $product ) {
		return new WP_Error( 'wooagent_product_not_found', __( 'Product not found.', 'wooagent-companion' ), array( 'status' => 404 ) );
	}

	$updated = array();

	if ( array_key_exists( 'name', $args ) ) {
		$product->set_name( $args['name'] );
		$updated[] = 'name';
	}
	if ( array_key_exists( 'description', $args ) ) {
		$product->set_description( $args['description'] );
		$updated[] = 'description';
	}
	if ( array_key_exists( 'short_description', $args ) ) {
		$product->set_short_description( $args['short_description'] );
		$updated[] = 'short_description';
	}
	if ( array_key_exists( 'regular_price', $args ) ) {
		$product->set_regular_price( $args['regular_price'] );
		$updated[] = 'regular_price';
	}
	if ( array_key_exists( 'sale_price', $args ) ) {
		$product->set_sale_price( $args['sale_price'] );
		$updated[] = 'sale_price';
	}
	if ( array_key_exists( 'status', $args ) ) {
		$product->set_status( $args['status'] );
		$updated[] = 'status';
	}

	$product->save();

	if ( array_key_exists( 'meta_title', $args ) ) {
		update_post_meta( $product->get_id(), '_yoast_wpseo_title', $args['meta_title'] );
		$updated[] = 'meta_title';
	}
	if ( array_key_exists( 'meta_description', $args ) ) {
		update_post_meta( $product->get_id(), '_yoast_wpseo_metadesc', $args['meta_description'] );
		$updated[] = 'meta_description';
	}

	return array(
		'id'             => $product->get_id(),
		'updated_fields' => $updated,
		'date_modified'  => $product->get_date_modified() ? $product->get_date_modified()->date( 'c' ) : '',
	);
}

function wooagent_product_to_summary( $product ): array {
	return array(
		'id'                       => $product->get_id(),
		'name'                     => $product->get_name(),
		'slug'                     => $product->get_slug(),
		'sku'                      => (string) $product->get_sku(),
		'status'                   => $product->get_status(),
		'type'                     => $product->get_type(),
		'regular_price'            => (string) $product->get_regular_price(),
		'sale_price'               => (string) $product->get_sale_price(),
		'on_sale'                  => $product->is_on_sale(),
		'description_length'       => strlen( (string) $product->get_description() ),
		'short_description_length' => strlen( (string) $product->get_short_description() ),
		'featured'                 => $product->is_featured(),
		'date_modified'            => $product->get_date_modified() ? $product->get_date_modified()->date( 'c' ) : '',
	);
}
