# CaptainBI Endpoints

| Domain | Command | Method | Path | Content Type | Required Inputs | Risk | Pagination | Summary |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| goods | `cbi goods edit-group` | POST | `/v1/open_goods/edit_goods_group` | `multipart/form-data` | `--group-name` | write_dangerous | none | 添加或编辑分组 |
| goods | `cbi goods groups` | GET | `/v1/open_goods_relevant/get_group_list` | - | `--page`, `--rows` | read | page_rows | 获取商品分组数据 |
| goods | `cbi goods items` | GET | `/v1/open_goods/get_goods_item_list` | - | `--channel`, `--page`, `--rows`, `--start-modified-time`, `--end-modified-time` | read | page_rows | 获取商品扩展数据 |
| goods | `cbi goods list` | GET | `/v1/open_goods/get_goods_list` | - | `--channel`, `--page`, `--rows`, `--start-modified-time`, `--end-modified-time` | read | page_rows | 获取商品基础数据 |
| goods | `cbi goods operators` | GET | `/v1/open_user/get_child_list` | - | `--page`, `--rows` | read | page_rows | 获取用户子账号数据（运营人员） |
| goods | `cbi goods set-group` | POST | `/v1/open_goods/set_goods_group` | `multipart/form-data` | `--goods-id`, `--group-id` | write_dangerous | none | 设置商品分组 |
| goods | `cbi goods set-operate-user` | POST | `/v1/open_goods/set_goods_operate_user` | `multipart/form-data` | `--channel`, `--goods-id`, `--operation-user-admin-id` | write_safe | none | 设置商品运营人员 |
| goods | `cbi goods set-shop-operation-mode` | POST | `/v1/open_user/set_channel_operation_mode` | `multipart/form-data` | `--channel` | write_safe | none | 设置店铺运营模式 |
| goods | `cbi goods shops` | GET | `/v1/open_user/get_channel_list` | - | `--page`, `--rows` | read | page_rows | 获取店铺数据 |
| goods | `cbi goods sites` | GET | `/v1/open_user/get_site_list` | - | - | read | none | 获取站点数据 |
| goods | `cbi goods tags` | GET | `/v1/open_goods_relevant/get_tags_list` | - | `--channel`, `--page`, `--rows` | read | page_rows | 获取商品标签数据 |
| sales | `cbi sales fbm-shipping-status` | GET | `/v1/open_order/get_fbm_order_ship_info` | - | `--channel`, `--feed-id` | read | none | 获取FBM订单发货信息上传状态 |
| sales | `cbi sales orders` | GET | `/v1/open_order/get_order_list` | - | `--channel`, `--page`, `--rows`, `--start-modified-time`, `--end-modified-time` | read | page_rows | 获取订单数据 |
| sales | `cbi sales refunds` | GET | `/v1/open_order/get_refund_list` | - | `--channel`, `--page`, `--rows`, `--start-modified-time`, `--end-modified-time` | read | page_rows | 获取退款明细 |
| sales | `cbi sales returns` | GET | `/v1/open_order/get_return_report` | - | `--channel`, `--page`, `--rows`, `--start-modified-time`, `--end-modified-time` | read | page_rows | 获取退货报告 |
| sales | `cbi sales upload-fbm-shipping` | POST | `/v1/open_order/upload_fbm_order_ship_info` | `multipart/form-data` | `--channel`, `data.data` | write_dangerous | none | 上传FBM订单发货信息 |
| finance | `cbi finance asin-daily` | GET | `/v1/open_goods_finance/get_analysis_by_order` | - | `--channel`, `--page`, `--rows`, `--report-date` | read | page_rows | 获取asin费用解析（下单维度） |
| finance | `cbi finance asin-daily-finance` | GET | `/v1/open_goods_finance/get_analysis_by_finance` | - | `--channel`, `--page`, `--rows`, `--report-date` | read | page_rows | 获取asin费用解析（财务维度） |
| finance | `cbi finance asin-monthly` | GET | `/v1/open_goods_finance/get_month_analysis_by_order` | - | `--channel`, `--page`, `--rows`, `--report-date` | read | page_rows | 获取asin月报（下单维度） |
| finance | `cbi finance asin-monthly-finance` | GET | `/v1/open_goods_finance/get_month_analysis_by_finance` | - | `--channel`, `--page`, `--rows`, `--report-date` | read | page_rows | 获取asin月报（财务维度） |
| finance | `cbi finance asin-transactions` | GET | `/v1/open_goods_finance/get_transaction_data` | - | `--channel`, `--page`, `--rows`, `--start-modified-time`, `--end-modified-time` | read | page_rows | 获取交易明细（商品） |
| finance | `cbi finance claims` | GET | `/v1/open_goods_finance/get_claim_list` | - | `--channel`, `--page`, `--rows`, `--start-modified-time`, `--end-modified-time` | read | page_rows | 获取FBA索赔信息 |
| finance | `cbi finance classify` | GET | `/v1/open_finance/get_classify` | - | `--channel` | read | page_rows | 获取信息（分类，支付对象，申请人） |
| finance | `cbi finance operating-expenses` | GET | `/v1/open_finance/operating_expenses_breakdown` | - | `--channel` | read | page_rows | 获取运营费用详情 |
| finance | `cbi finance payment-record` | GET | `/v1/open_finance/get_payment_record` | - | `--channel` | read | page_rows | 获取回款记录 |
| finance | `cbi finance set-cost` | POST | `/v1/open_finance/set_goods_cost` | `multipart/form-data` | `--channel`, `data.data` | write_dangerous | none | 产品成本设置 |
| finance | `cbi finance set-rule` | POST | `/v1/open_finance/set_rule` | `multipart/form-data` | `--channel` | write_dangerous | none | 运营费用设置 |
| finance | `cbi finance store-daily` | GET | `/v1/open_channel_finance/get_analysis_by_order` | - | `--channel`, `--page`, `--rows`, `--report-date` | read | page_rows | 获取店铺费用解析（下单维度） |
| finance | `cbi finance store-daily-finance` | GET | `/v1/open_channel_finance/get_analysis_by_finance` | - | `--channel`, `--page`, `--rows`, `--report-date` | read | page_rows | 获取店铺费用解析（财务维度） |
| finance | `cbi finance store-monthly` | GET | `/v1/open_channel_finance/get_month_analysis_by_order` | - | `--channel`, `--page`, `--rows`, `--report-date` | read | page_rows | 获取店铺月报（下单维度） |
| finance | `cbi finance store-monthly-finance` | GET | `/v1/open_channel_finance/get_month_analysis_by_finance` | - | `--channel`, `--report-date` | read | page_rows | 获取店铺月报（财务维度） |
| finance | `cbi finance store-performance` | GET | `/v1/open_finance/get_store_performance` | - | `--channel` | read | page_rows | 获取店铺绩效 |
| finance | `cbi finance store-transactions` | GET | `/v1/open_channel_finance/get_transaction_data` | - | `--channel`, `--page`, `--rows`, `--start-report-time`, `--end-report-time` | read | page_rows | 获取交易明细（店铺） |
| finance | `cbi finance storewide-performance` | GET | `/v1/open_finance/get_storewide_performance` | - | `--channel` | read | page_rows | 获取全店铺绩效 |
| finance | `cbi finance vat` | GET | `/v1/open_channel_finance/get_vat_report_list` | - | `--channel`, `--page`, `--rows`, `--start-modified-time`, `--end-modified-time` | read | page_rows | 获取vat报告 |
| fba | `cbi fba abnormal-fee` | GET | `/v1/open_fba/abnormal_distribution_fee` | - | `--channel` | read | page_rows | 获取异常分配费用 |
| fba | `cbi fba asin-monitor` | GET | `/v1/open_fba/get_amazon_asin_monitor_list` | - | `--channel` | read | page_rows | 获取被跟卖监控列表 |
| fba | `cbi fba inventory` | GET | `/v1/open_fba/inventory_list` | - | `--channel`, `--page`, `--rows`, `--start-modified-time`, `--end-modified-time` | read | page_rows | 获取库存列表 |
| fba | `cbi fba shipments` | GET | `/v1/open_fba/get_amazon_shipment_list` | - | `--channel` | read | page_rows | 获取FBA货件管理 |
| fba | `cbi fba storage-fee` | GET | `/v1/open_finance/get_amazon_finance_storage_fee_report_list` | - | `--channel` | read | page_rows | 获取FBA仓储费用 |
| fba | `cbi fba sync-shipment` | POST | `/v1/open_fba/sync_shipment` | `multipart/form-data` | `--channel`, `--shipment-ids` | sync_trigger | none | 同步FBA货件 |
| ads | `cbi ads advertise` | GET | `/v1/open_cpc/advertise` | - | `--channel` | read | page_rows | 广告 |
| ads | `cbi ads advertise-campaign` | GET | `/v1/open_cpc/advertise_campaign` | - | `--channel` | read | page_rows | 广告活动 |
| ads | `cbi ads advertise-campaign-report` | GET | `/v1/open_cpc/advertise_campaign_report` | - | `--channel`, `--report-date` | read | page_rows | 广告活动报告 |
| ads | `cbi ads advertise-expression` | GET | `/v1/open_cpc/advertise_expression` | - | `--channel` | read | page_rows | 广告表达式 |
| ads | `cbi ads advertise-group` | GET | `/v1/open_cpc/advertise_group` | - | `--channel` | read | page_rows | 广告组 |
| ads | `cbi ads advertise-group-report` | GET | `/v1/open_cpc/advertise_group_report` | - | `--channel`, `--report-date` | read | page_rows | 广告组报告 |
| ads | `cbi ads advertise-keyword` | GET | `/v1/open_cpc/advertise_keyword` | - | `--channel` | read | page_rows | 广告关键词 |
| ads | `cbi ads advertise-keyword-report` | GET | `/v1/open_cpc/advertise_keyword_report` | - | `--channel`, `--report-date` | read | page_rows | 关键词投放报告 |
| ads | `cbi ads advertise-mix` | GET | `/v1/open_cpc/advertise_mix` | - | `--channel` | read | page_rows | 广告组合 |
| ads | `cbi ads advertise-mix-report` | GET | `/v1/open_cpc/advertise_mix_report` | - | `--channel`, `--report-date` | read | page_rows | 广告组合报告 |
| ads | `cbi ads advertise-placement` | GET | `/v1/open_cpc/advertise_placement` | - | `--channel` | read | page_rows | 广告投放 |
| ads | `cbi ads advertise-report` | GET | `/v1/open_cpc/advertise_report` | - | `--channel`, `--report-date` | read | page_rows | 广告报告 |
| ads | `cbi ads negative-advertise-expression` | GET | `/v1/open_cpc/negative_advertise_expression` | - | `--channel` | read | page_rows | 否定广告表达式 |
| ads | `cbi ads negative-advertise-keyword` | GET | `/v1/open_cpc/negative_advertise_keyword` | - | `--channel` | read | page_rows | 否定广告关键词 |
| ads | `cbi ads negative-advertise-placement` | GET | `/v1/open_cpc/negative_advertise_placement` | - | `--channel` | read | page_rows | 否定广告投放 |
| ads | `cbi ads product-targeting-report` | GET | `/v1/open_cpc/product_targeting_report` | - | `--channel`, `--report-date` | read | page_rows | 商品投放报告 |
| ads | `cbi ads search-term-keywords-report` | GET | `/v1/open_cpc/search_term_keywords_report` | - | `--channel`, `--report-date` | read | page_rows | 搜索词（关键词）报告 |
| ads | `cbi ads search-term-placement-report` | GET | `/v1/open_cpc/search_term_placement_report` | - | `--channel`, `--report-date` | read | page_rows | 搜索词（投放）报告 |
| monitor | `cbi monitor bad-review-summary` | GET | `/v1/open_goods/get_monitoring_list` | - | `--channel`, `--page`, `--rows`, `--start-modified-time`, `--end-modified-time` | read | page_rows | 获取监控列表 |
| monitor | `cbi monitor business-report` | GET | `/v1/open_goods/get_business_report` | - | `--channel`, `--page`, `--rows`, `--start-create-time`, `--end-create-time` | read | page_rows | 获取业务报告 |
| monitor | `cbi monitor feedback` | GET | `/v1/open_goods/get_feedback_monitoring` | - | `--channel`, `--page`, `--rows` | read | page_rows | 获取Feedback监控 |
| monitor | `cbi monitor followup` | GET | `/v1/open_goods/get_followup_monitoring_list` | - | `--channel`, `--page`, `--rows` | read | page_rows | 被跟卖监控列表 |
| monitor | `cbi monitor hijacked-record` | GET | `/v1/open_goods/get_hijacked_record` | - | `--channel`, `--page`, `--rows`, `--monitor-id` | read | page_rows | 获取被跟卖记录 |
| monitor | `cbi monitor reviews` | GET | `/v1/open_goods/get_reviews_list` | - | `--channel`, `--page`, `--rows` | read | page_rows | 获取review列表 |
